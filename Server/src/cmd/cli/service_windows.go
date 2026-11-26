package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"golang.org/x/term"
)

const (
	serviceName    = "WeirdStreamer"
	firewallRule   = "WeirdStreamer server"
	serviceUsage   = "Usage: cli service install [--user <DOMAIN\\name>] | remove | start | stop | status"
	logonAsService = "SeServiceLogonRight"

	// How to start and stop the server's service, for messages.
	startHint = "cli service start"
	stopHint  = "cli service stop"
)

// handleServiceCommand installs, removes, starts, stops or shows the Windows service that runs
// this package's bin\server.exe: as the user who installs it (Windows asks for the account's
// password to start it; it's handed to Windows and kept nowhere here), from the package folder,
// started at boot once the network is up, and started again after it stops by itself (a crash, or
// cli server restart). Installing also lets the server through Windows Firewall. An elevated
// (administrator) prompt is needed for install, remove, start and stop.
func handleServiceCommand(args []string) {
	if len(args) < 1 {
		fmt.Println(serviceUsage)
		os.Exit(2)
	}
	var err error
	switch args[0] {
	case "install":
		account := ""
		if len(args) == 3 && args[1] == "--user" {
			account = args[2]
		} else if len(args) != 1 {
			fmt.Println(serviceUsage)
			os.Exit(2)
		}
		err = installService(account)
	case "remove":
		err = removeService()
	case "start":
		err = startService()
	case "stop":
		err = stopService()
	case "status":
		err = serviceStatus()
	default:
		fmt.Println(serviceUsage)
		os.Exit(2)
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		err = errors.New("access is denied: run this in an administrator PowerShell (Run as administrator)")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// packageServer is the server beside this cli (bin\server.exe), and the package folder.
func packageServer() (exe, dir string, err error) {
	self, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	exe = filepath.Join(filepath.Dir(self), "server.exe")
	if _, err := os.Stat(exe); err != nil {
		return "", "", fmt.Errorf("no server.exe beside cli.exe: %w", err)
	}
	dir = filepath.Dir(filepath.Dir(exe))
	if _, err := os.Stat(filepath.Join(dir, "config.yml")); err != nil {
		return "", "", fmt.Errorf("no config.yml in %s (the service runs the server there): write one first, cli config init", dir)
	}
	return exe, dir, nil
}

func installService(account string) error {
	exe, dir, err := packageServer()
	if err != nil {
		return err
	}
	if account == "" {
		me, err := user.Current()
		if err != nil {
			return err
		}
		account = me.Username // DOMAIN\name
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if s, err := m.OpenService(serviceName); err == nil {
		s.Close()
		return fmt.Errorf("the %s service is already installed (cli service remove first)", serviceName)
	}
	fmt.Printf("The service runs the server as %s. Windows needs that account's password to start it.\n", account)
	fmt.Print("Password: ")
	var password []byte
	if term.IsTerminal(int(os.Stdin.Fd())) {
		password, err = term.ReadPassword(int(os.Stdin.Fd()))
	} else { // piped in (a script): its first line
		var line string
		line, err = bufio.NewReader(os.Stdin).ReadString('\n')
		if err == io.EOF && line != "" {
			err = nil
		}
		password = []byte(strings.TrimRight(line, "\r\n"))
	}
	fmt.Println()
	if err != nil {
		return err
	}
	if err := checkLogon(account, string(password)); err != nil {
		return err
	}
	if err := grantRight(account, logonAsService); err != nil {
		return fmt.Errorf("letting %s run services: %w", account, err)
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName:      "Weird Streamer",
		Description:      "Weird Streamer media server (" + dir + ")",
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true, // after the network and the shares it reaches
		ServiceStartName: account,
		Password:         string(password),
	})
	if err != nil {
		return err
	}
	defer s.Close()
	// Started again when it stops by itself (cli server restart exits with status 3, a crash):
	// at once, then after 10 s, then each minute; the count starts over after a day.
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: time.Minute},
	}, 24*60*60); err != nil {
		return err
	}
	if err := s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return err
	}
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+firewallRule).Run()
	if out, err := exec.Command("netsh", "advfirewall", "firewall", "add", "rule", "name="+firewallRule,
		"dir=in", "action=allow", "program="+exe, "enable=yes").CombinedOutput(); err != nil {
		fmt.Printf("Warning: couldn't let the server through Windows Firewall: %v %s\n", err, out)
	}
	fmt.Printf("Installed the %s service (%s, as %s). Starting it…\n", serviceName, exe, account)
	if err := s.Start(); err != nil {
		return fmt.Errorf("installed, but it didn't start: %w", err)
	}
	return waitFor(s, svc.Running)
}

func removeService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("no %s service is installed", serviceName)
	}
	defer s.Close()
	if st, err := s.Query(); err == nil && st.State != svc.Stopped {
		if _, err := s.Control(svc.Stop); err == nil {
			waitFor(s, svc.Stopped)
		}
	}
	if err := s.Delete(); err != nil {
		return err
	}
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+firewallRule).Run()
	fmt.Printf("Removed the %s service.\n", serviceName)
	return nil
}

func startService() error {
	return withService(func(s *mgr.Service) error {
		if err := s.Start(); err != nil {
			return err
		}
		return waitFor(s, svc.Running)
	})
}

func stopService() error {
	return withService(func(s *mgr.Service) error {
		if _, err := s.Control(svc.Stop); err != nil {
			return err
		}
		return waitFor(s, svc.Stopped)
	})
}

func serviceStatus() error {
	return withService(func(s *mgr.Service) error {
		st, err := s.Query()
		if err != nil {
			return err
		}
		c, err := s.Config()
		if err != nil {
			return err
		}
		fmt.Printf("%s: %s (runs %s as %s)\n", serviceName, stateName(st.State), c.BinaryPathName, c.ServiceStartName)
		return nil
	})
}

func withService(do func(*mgr.Service) error) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("no %s service is installed (cli service install)", serviceName)
	}
	defer s.Close()
	return do(s)
}

// waitFor waits up to 30 s for the service to reach state.
func waitFor(s *mgr.Service, state svc.State) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == state {
			fmt.Printf("%s: %s\n", serviceName, stateName(state))
			return nil
		}
		if state == svc.Running && st.State == svc.Stopped {
			return errors.New("it stopped again: see the newest server-*.log in the logs folder")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("still %s after 30 s", stateName(st.State))
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func stateName(s svc.State) string {
	switch s {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "starting"
	case svc.StopPending:
		return "stopping"
	case svc.Running:
		return "running"
	}
	return fmt.Sprintf("state %d", s)
}

// checkLogon checks the account's password before Windows is given it, so a wrong one is caught
// here rather than as a service that won't start.
func checkLogon(account, password string) error {
	domain, name := ".", account
	for i := len(account) - 1; i >= 0; i-- {
		if account[i] == '\\' {
			domain, name = account[:i], account[i+1:]
			break
		}
	}
	var token windows.Token
	const logonNetwork, providerDefault = 3, 0
	u, _ := windows.UTF16PtrFromString(name)
	d, _ := windows.UTF16PtrFromString(domain)
	p, _ := windows.UTF16PtrFromString(password)
	if r, _, err := logonUser.Call(uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(p)),
		logonNetwork, providerDefault, uintptr(unsafe.Pointer(&token))); r == 0 {
		return fmt.Errorf("signing in as %s: %w", account, err)
	}
	token.Close()
	return nil
}

var (
	advapi32              = windows.NewLazySystemDLL("advapi32.dll")
	logonUser             = advapi32.NewProc("LogonUserW")
	lsaOpenPolicy         = advapi32.NewProc("LsaOpenPolicy")
	lsaAddAccountRights   = advapi32.NewProc("LsaAddAccountRights")
	lsaClose              = advapi32.NewProc("LsaClose")
	lsaNtStatusToWinError = advapi32.NewProc("LsaNtStatusToWinError")
)

// lsaString is LSA_UNICODE_STRING.
type lsaString struct {
	Length, MaximumLength uint16
	Buffer                *uint16
}

func newLsaString(s string) lsaString {
	p, _ := windows.UTF16FromString(s)
	return lsaString{Length: uint16((len(p) - 1) * 2), MaximumLength: uint16(len(p) * 2), Buffer: &p[0]}
}

// grantRight gives an account a user right (a privilege such as SeServiceLogonRight) through the
// local security policy; having it already is fine.
func grantRight(account, right string) error {
	sid, _, _, err := windows.LookupSID("", account)
	if err != nil {
		return err
	}
	const policyCreateAccount, policyLookupNames = 0x10, 0x800
	var attrs [12]uintptr // LSA_OBJECT_ATTRIBUTES, zeroed but for its length
	attrs[0] = unsafe.Sizeof(attrs)
	var policy uintptr
	if st, _, _ := lsaOpenPolicy.Call(0, uintptr(unsafe.Pointer(&attrs[0])), policyCreateAccount|policyLookupNames,
		uintptr(unsafe.Pointer(&policy))); st != 0 {
		return ntError(st)
	}
	defer lsaClose.Call(policy)
	r := newLsaString(right)
	if st, _, _ := lsaAddAccountRights.Call(policy, uintptr(unsafe.Pointer(sid)), uintptr(unsafe.Pointer(&r)), 1); st != 0 {
		return ntError(st)
	}
	return nil
}

func ntError(status uintptr) error {
	code, _, _ := lsaNtStatusToWinError.Call(status)
	return windows.Errno(code)
}
