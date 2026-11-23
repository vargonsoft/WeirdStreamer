package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/weirdstreamer/server/internal/backup"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

// handleBackupCommand: cli backup now | list | restore <file> [--with-config]. Handled before the
// CLI opens the databases, which a restore replaces.
func handleBackupCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Missing backup subcommand: now, list or restore <file>")
		os.Exit(1)
	}
	switch args[0] {
	case "now":
		db, err := database.New(config.GlobalConfig.Database.UserPath, config.GlobalConfig.Database.MediaPath)
		if err != nil {
			fmt.Printf("Error: can't open the databases: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		info, err := backup.Create(db, version)
		if err != nil {
			fmt.Printf("Error: the backup failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Made %s (%s) in %s\n", info.Name, backup.Size(info.Size), backup.Dir())
	case "list":
		list := backup.List(backup.Dir())
		if len(list) == 0 {
			fmt.Printf("No backups in %s\n", backup.Dir())
			return
		}
		fmt.Printf("Backups in %s, newest first:\n", backup.Dir())
		for _, b := range list {
			fmt.Printf("  %s  %s  %s\n", b.Name, b.Created.Format("2006-01-02 15:04"), backup.Size(b.Size))
		}
	case "restore":
		if len(args) < 2 {
			fmt.Println("Usage: cli backup restore <file> [--with-config]   (a name from cli backup list, or a path)")
			os.Exit(1)
		}
		withConfig := len(args) > 2 && args[2] == "--with-config"
		if serverRunning() {
			fmt.Println("Error: the server is running: stop it first (sudo systemctl stop weirdstreamer, or Ctrl-C), then restore.")
			os.Exit(1)
		}
		file := backup.Resolve(args[1])
		m, err := backup.ReadManifest(file)
		if err != nil {
			fmt.Printf("Error: %s isn't a backup this server made: %v\n", file, err)
			os.Exit(1)
		}
		aside, err := backup.Restore(file, withConfig)
		if err != nil {
			fmt.Printf("Error: the restore failed: %v\n", err)
			if aside != "" {
				fmt.Printf("The files it replaced are in %s.\n", aside)
			}
			os.Exit(1)
		}
		what := "The databases"
		if withConfig {
			what += " and config.yml"
		}
		fmt.Printf("%s are back as they were at %s (made by %s). The files they replaced are in %s.\n",
			what, m.Created.Format("2006-01-02 15:04"), m.Version, aside)
		fmt.Println("Start the server again.")
	default:
		fmt.Printf("Unknown backup subcommand: %s (now, list or restore)\n", args[0])
		os.Exit(1)
	}
}

// serverRunning reports whether a server answers on this config's port here.
func serverRunning() bool {
	port := config.GlobalConfig.Server.Port
	if port == 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
