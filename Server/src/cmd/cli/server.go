package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// handleServerCommand is `cli server stop|restart [--force]`: it asks the server running here with
// this config to stop, or to restart (it exits with handlers.RestartExitCode, and its service or
// live.sh starts it again), through POST /local/stop|restart with local.key. No root needed, so the
// server's own account can do it. While anyone is watching it lists the streams and exits 2 unless
// --force.
func handleServerCommand(args []string, configPath string) {
	if len(args) == 0 || (args[0] != "stop" && args[0] != "restart") {
		fmt.Fprintln(os.Stderr, "Usage: cli server stop|restart [--force]")
		os.Exit(1)
	}
	restart, force := args[0] == "restart", false
	for _, a := range args[1:] {
		if a != "--force" {
			fmt.Fprintf(os.Stderr, "Unknown option: %s\n", a)
			os.Exit(1)
		}
		force = true
	}
	if !serverRunning() {
		fmt.Fprintf(os.Stderr, "No server answers on port %d here.\n", config.GlobalConfig.Server.Port)
		os.Exit(1)
	}
	keyFile := filepath.Join(filepath.Dir(configPath), "local.key")
	key, err := os.ReadFile(keyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't read %s (written by the server at each start, for its account only): %v\n", keyFile, err)
		os.Exit(1)
	}
	query := ""
	if force {
		query = "?force=1"
	}
	status, body, err := localPost("/local/"+args[0]+query, strings.TrimSpace(string(key)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't reach the server: %v\n", err)
		os.Exit(1)
	}
	switch status {
	case http.StatusAccepted:
	case http.StatusConflict:
		var s struct {
			Active  int `json:"active"`
			Streams []struct {
				User, Client, State, Mode string
			} `json:"streams"`
		}
		json.Unmarshal(body, &s)
		fmt.Fprintf(os.Stderr, "%d stream(s) active; not stopping (--force to anyway):\n", s.Active)
		for _, st := range s.Streams {
			if st.State != "idle" {
				fmt.Fprintf(os.Stderr, "  %s on %s: %s (%s)\n", st.User, st.Client, st.State, st.Mode)
			}
		}
		os.Exit(2)
	case http.StatusNotFound:
		fmt.Fprintln(os.Stderr, "The server refused: its local.key doesn't match (another config?), or it's too old to stop this way.")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "The server answered %d: %s\n", status, strings.TrimSpace(string(body)))
		os.Exit(1)
	}

	// Wait for it to go, then (restart) to come back.
	deadline := time.Now().Add(20 * time.Second)
	for serverRunning() {
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "It's still answering after 20 seconds.")
			os.Exit(1)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !restart {
		fmt.Println("Stopped. A service won't start it again by itself: " + startHint + ".")
		return
	}
	deadline = time.Now().Add(60 * time.Second)
	for !serverRunning() {
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "Stopped, but not started again within a minute: is it run by a service or live.sh?")
			os.Exit(1)
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("Restarted.")
}

// localPost posts to the server on this machine, over HTTPS when it serves that (its certificate
// isn't checked: the connection never leaves the machine, and the key is what's secret), else HTTP.
func localPost(path, key string) (int, []byte, error) {
	addr := "127.0.0.1:" + strconv.Itoa(config.GlobalConfig.Server.Port)
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // loopback only
	}}
	var lastErr error
	for _, scheme := range []string{"https", "http"} {
		req, err := http.NewRequest(http.MethodPost, scheme+"://"+addr+path, nil)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("X-Local-Key", key)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return resp.StatusCode, body, nil
	}
	return 0, nil, lastErr
}
