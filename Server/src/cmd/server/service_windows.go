package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// asService is set when Windows' service manager started the server (cli service install).
var asService bool

// runService runs serve as a Windows service when the service manager started this process, and
// returns true once it has stopped; false when started some other way (a console). The service
// works from the package's folder (bin\server.exe's parent), as setup.sh's systemd unit does: a
// service starts in System32, and config.yml and the data folders are relative to the package.
// Windows' Stop (and shutting down) stops it cleanly; cli server stop ends it as stopped, and cli
// server restart exits with RestartExitCode, which the service's recovery actions start again.
func runService(serve func(context.Context)) bool {
	is, err := svc.IsWindowsService()
	if err != nil || !is {
		return false
	}
	asService = true
	if exe, err := os.Executable(); err == nil {
		os.Chdir(filepath.Dir(filepath.Dir(exe)))
	}
	if err := svc.Run("WeirdStreamer", &service{serve: serve}); err != nil {
		log.Printf("The service didn't run: %v", err)
		os.Exit(1)
	}
	return true
}

type service struct{ serve func(context.Context) }

func (s *service) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		s.serve(ctx)
		close(done)
	}()
	running := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	status <- running
	for {
		select {
		case <-done:
			return false, 0 // cli server stop
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 20000}
				cancel()
				<-done
				return false, 0
			}
		}
	}
}

// serviceLog sends the log of a service, which has no console, to a file of its own in the logs
// folder for each start, server-<start time>.log, keeping the last 20, as live.sh does. Panics
// (written to stderr) go there too.
func serviceLog(logsPath string) {
	if !asService || logsPath == "" {
		return
	}
	if err := os.MkdirAll(logsPath, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(logsPath, "server-"+time.Now().Format("2006-01-02_150405")+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	log.SetOutput(f)
	os.Stdout, os.Stderr = f, f
	windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(f.Fd()))
	windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
	old, _ := filepath.Glob(filepath.Join(logsPath, "server-*.log"))
	sort.Strings(old) // by start time: the names sort as their dates
	for len(old) > 20 {
		os.Remove(old[0])
		old = old[1:]
	}
}
