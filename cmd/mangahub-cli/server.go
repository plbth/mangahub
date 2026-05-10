package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Check server status",
	}
	cmd.AddCommand(newServerPingCmd())
	cmd.AddCommand(newServerStatusCmd())
	return cmd
}

func newServerPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Check if the HTTP server is reachable",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newHTTPClient()
			start := time.Now()
			if err := client.ping(); err != nil {
				return err
			}
			fmt.Printf("HTTP API is reachable (%s)\n", time.Since(start).Round(time.Millisecond))
			return nil
		},
	}
}

func newServerStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show a basic server status summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newHTTPClient()
			if err := client.ping(); err != nil {
				return err
			}
			fmt.Println("MangaHub API Status: healthy")
			fmt.Printf("Base URL: %s\n", cfg.APIBaseURL)
			fmt.Println("HTTP: online")
			fmt.Println("TCP / UDP / WebSocket / gRPC: managed by the server orchestrator")
			return nil
		},
	}
}
