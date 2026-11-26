package media

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

// separateOutput starts cmd, whose last argument is ffmpeg's output "pipe:1", writing that output
// to a pipe of its own and its stdout and stderr to another. Windows can't hand a child a third
// pipe (exec's ExtraFiles), so ffmpeg writes to a named pipe made for it, as to a file: one
// instance, inbound only (its clients need write access, which only this account, admins and
// SYSTEM have), local clients only.
func separateOutput(cmd *exec.Cmd) (output, messages io.ReadCloser, err error) {
	id := make([]byte, 16)
	rand.Read(id)
	name := `\\.\pipe\weirdstreamer-` + hex.EncodeToString(id)
	name16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, err
	}
	h, err := windows.CreateNamedPipe(name16,
		windows.PIPE_ACCESS_INBOUND|windows.FILE_FLAG_FIRST_PIPE_INSTANCE,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		1, 0, 1<<20, 0, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the output pipe: %w", err)
	}
	msgR, msgW, err := os.Pipe()
	if err != nil {
		windows.CloseHandle(h)
		return nil, nil, fmt.Errorf("failed to create the message pipe: %w", err)
	}
	// -y: the pipe exists, and ffmpeg would otherwise ask whether to overwrite it.
	last := len(cmd.Args) - 1
	cmd.Args = append(cmd.Args[:last:last], "-y", name)
	cmd.Stdout, cmd.Stderr = msgW, msgW
	err = cmd.Start()
	msgW.Close()
	if err != nil {
		windows.CloseHandle(h)
		msgR.Close()
		return nil, nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	out := &namedPipeOutput{h: h, connected: make(chan error, 1)}
	go func() {
		err := windows.ConnectNamedPipe(h, nil)
		if errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
			err = nil
		}
		out.isConnected.Store(true)
		out.connected <- err
	}()
	// An ffmpeg that ends before opening its output (a bad input) never connects: then this
	// connects and leaves at once, so the wait ends and the reader sees the end.
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err == nil {
		go func() {
			windows.WaitForSingleObject(process, windows.INFINITE)
			windows.CloseHandle(process)
			if out.isConnected.Load() {
				return
			}
			if c, err := windows.CreateFile(name16, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0); err == nil {
				windows.CloseHandle(c)
			}
		}()
	}
	return out, msgR, nil
}

// namedPipeOutput reads a named pipe once its client (ffmpeg) has connected; the client's end
// (it exits or closes it) is io.EOF.
type namedPipeOutput struct {
	h           windows.Handle
	connected   chan error
	isConnected atomic.Bool
	wait        sync.Once
	err         error
	closed      sync.Once
}

func (p *namedPipeOutput) Read(b []byte) (int, error) {
	p.wait.Do(func() { p.err = <-p.connected })
	if p.err != nil {
		return 0, p.err
	}
	if len(b) == 0 {
		return 0, nil
	}
	var n uint32
	err := windows.ReadFile(p.h, b, &n, nil)
	if errors.Is(err, windows.ERROR_BROKEN_PIPE) || (err == nil && n == 0) {
		return 0, io.EOF
	}
	return int(n), err
}

func (p *namedPipeOutput) Close() error {
	var err error
	p.closed.Do(func() { err = windows.CloseHandle(p.h) })
	return err
}
