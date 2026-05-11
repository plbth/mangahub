package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/plbth/mangahub/pkg/models"
	"github.com/spf13/cobra"
)

func newNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "UDP notification commands",
	}
	cmd.AddCommand(newNotifySubscribeCmd())
	cmd.AddCommand(newNotifySendCmd())
	return cmd
}

func newNotifySubscribeCmd() *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "subscribe",
		Short: "Subscribe to UDP notifications",
		RunE: func(cmd *cobra.Command, args []string) error {
			udpAddr, err := net.ResolveUDPAddr("udp", addr)
			if err != nil {
				return fmt.Errorf("resolve UDP server address %s: %w", addr, err)
			}

			conn, err := net.DialUDP("udp", nil, udpAddr)
			if err != nil {
				return fmt.Errorf("connect to UDP notification server at %s: %w", addr, err)
			}
			defer conn.Close()

			if _, err := conn.Write([]byte("SUBSCRIBE")); err != nil {
				return fmt.Errorf("subscribe to UDP notifications: %w", err)
			}

			fmt.Printf("Subscribed to UDP notification server at %s. Waiting for notifications...\n", addr)

			buf := make([]byte, 4096)
			for {
				n, err := conn.Read(buf)
				if err != nil {
					return fmt.Errorf("read UDP notification: %w", err)
				}

				payload := strings.TrimSpace(string(buf[:n]))
				var notification models.Notification
				if err := json.Unmarshal([]byte(payload), &notification); err == nil && notification.Message != "" {
					raw, err := json.MarshalIndent(notification, "", "  ")
					if err != nil {
						return fmt.Errorf("format UDP notification: %w", err)
					}
					fmt.Println(string(raw))
					continue
				}

				fmt.Println(payload)
			}
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:9091", "UDP notification server address")
	return cmd
}

func newNotifySendCmd() *cobra.Command {
	var addr, mangaID, message string

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send one UDP notification",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(mangaID) == "" {
				return fmt.Errorf("--manga-id is required")
			}
			if strings.TrimSpace(message) == "" {
				return fmt.Errorf("--message is required")
			}

			udpAddr, err := net.ResolveUDPAddr("udp", addr)
			if err != nil {
				return fmt.Errorf("resolve UDP server address %s: %w", addr, err)
			}

			conn, err := net.DialUDP("udp", nil, udpAddr)
			if err != nil {
				return fmt.Errorf("connect to UDP notification server at %s: %w", addr, err)
			}
			defer conn.Close()

			payload := fmt.Sprintf("NOTIFY %s|%s", strings.TrimSpace(mangaID), strings.TrimSpace(message))
			if _, err := conn.Write([]byte(payload)); err != nil {
				return fmt.Errorf("send UDP notification: %w", err)
			}

			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			buf := make([]byte, 1024)
			if n, err := conn.Read(buf); err == nil {
				reply := strings.TrimSpace(string(buf[:n]))
				if strings.HasPrefix(reply, "ERR ") {
					return fmt.Errorf("UDP server rejected notification: %s", reply)
				}
			}

			fmt.Printf("Sent UDP notification to %s: manga=%s message=%q\n", addr, mangaID, message)
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:9091", "UDP notification server address")
	cmd.Flags().StringVar(&mangaID, "manga-id", "", "manga id")
	cmd.Flags().StringVar(&message, "message", "", "notification message")
	_ = cmd.MarkFlagRequired("manga-id")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}
