//go:build unix

package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// lowPriorityClass is Windows' way; elsewhere nice and ionice do it (nil: use them).
func lowPriorityClass(ctx context.Context, name string, args ...string) *exec.Cmd { return nil }

// windowsMemory is only read on Windows.
func windowsMemory() int64 { return 0 }

// separateOutput starts cmd, whose last argument is ffmpeg's output "pipe:1", writing that output
// to a pipe of its own (fd 3: exec's ExtraFiles) and its stdout and stderr to another.
func separateOutput(cmd *exec.Cmd) (output, messages io.ReadCloser, err error) {
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the output pipe: %w", err)
	}
	msgR, msgW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		return nil, nil, fmt.Errorf("failed to create the message pipe: %w", err)
	}
	cmd.Args[len(cmd.Args)-1] = "pipe:3"
	cmd.ExtraFiles = []*os.File{outW}
	cmd.Stdout, cmd.Stderr = msgW, msgW
	err = cmd.Start()
	// ffmpeg has its own copies of the write ends: closing these lets the readers see its end.
	outW.Close()
	msgW.Close()
	if err != nil {
		outR.Close()
		msgR.Close()
		return nil, nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	return outR, msgR, nil
}
