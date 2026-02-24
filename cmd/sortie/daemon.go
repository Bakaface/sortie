package main

import (
	"fmt"

	"github.com/aface/sortie/internal/daemon"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the Sortie daemon",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		foreground, _ := cmd.Flags().GetBool("foreground")
		return daemon.Start(cfg, foreground)
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon gracefully",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Stop(cfg)
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if daemon is running",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, pid, err := daemon.Status(cfg)
		if err != nil {
			return err
		}
		if running {
			fmt.Printf("Daemon is running (PID: %d)\n", pid)
		} else {
			fmt.Println("Daemon is not running")
		}
		return nil
	},
}
