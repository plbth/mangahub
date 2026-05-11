package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/plbth/mangahub/pkg/models"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "TCP progress sync commands",
	}
	cmd.AddCommand(newSyncMonitorCmd())
	cmd.AddCommand(newSyncSendCmd())
	return cmd
}

func newSyncMonitorCmd() *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Listen for TCP progress updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return fmt.Errorf("connect to TCP sync server at %s: %w", addr, err)
			}
			defer conn.Close()

			fmt.Printf("Connected to TCP sync server at %s. Waiting for JSON updates...\n", addr)

			decoder := json.NewDecoder(conn)
			for {
				var update models.ProgressUpdate
				if err := decoder.Decode(&update); err != nil {
					return fmt.Errorf("read TCP update: %w", err)
				}

				raw, err := json.MarshalIndent(update, "", "  ")
				if err != nil {
					return fmt.Errorf("format TCP update: %w", err)
				}
				fmt.Println(string(raw))
			}
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:9090", "TCP sync server address")
	return cmd
}

func newSyncSendCmd() *cobra.Command {
	var addr, userID, mangaID string
	var chapter int

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send one TCP progress update",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(userID) == "" {
				return fmt.Errorf("--user-id is required")
			}
			if strings.TrimSpace(mangaID) == "" {
				return fmt.Errorf("--manga-id is required")
			}
			if chapter <= 0 {
				return fmt.Errorf("--chapter must be greater than 0")
			}

			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return fmt.Errorf("connect to TCP sync server at %s: %w", addr, err)
			}
			defer conn.Close()

			update := models.ProgressUpdate{
				UserID:  userID,
				MangaID: mangaID,
				Chapter: chapter,
			}
			if err := json.NewEncoder(conn).Encode(update); err != nil {
				return fmt.Errorf("send TCP update: %w", err)
			}

			fmt.Printf("Sent progress update to %s: user=%s manga=%s chapter=%d\n",
				addr, userID, mangaID, chapter)
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:9090", "TCP sync server address")
	cmd.Flags().StringVar(&userID, "user-id", "", "user id")
	cmd.Flags().StringVar(&mangaID, "manga-id", "", "manga id")
	cmd.Flags().IntVar(&chapter, "chapter", 0, "chapter number")
	_ = cmd.MarkFlagRequired("user-id")
	_ = cmd.MarkFlagRequired("manga-id")
	_ = cmd.MarkFlagRequired("chapter")
	return cmd
}
