package main

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/plbth/mangahub/pkg/models"
	"github.com/spf13/cobra"
)

func newChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "WebSocket chat commands",
	}

	cmd.AddCommand(newChatJoinCmd())
	return cmd
}

func newChatJoinCmd() *cobra.Command {
	var mangaID, name, token string
	var guest bool

	cmd := &cobra.Command{
		Use:   "join",
		Short: "Join the WebSocket chat from this terminal",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(token) == "" {
				token = strings.TrimSpace(cfg.Token)
			}
			if strings.TrimSpace(token) == "" {
				guest = true
			}
			if guest && strings.TrimSpace(name) == "" {
				name = defaultGuestName()
			}

			wsURL, err := buildWebSocketURL(cfg.APIBaseURL, mangaID, name, guest)
			if err != nil {
				return err
			}

			header := http.Header{}
			if !guest && strings.TrimSpace(token) != "" {
				header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
			}

			conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
			if err != nil {
				return fmt.Errorf("connect websocket: %w", err)
			}
			defer conn.Close()

			room := "general"
			if strings.TrimSpace(mangaID) != "" {
				room = strings.TrimSpace(mangaID)
			}
			fmt.Printf("Connected to MangaHub chat room %q.\n", room)
			fmt.Println("Type a message and press Enter. Use Ctrl+C or /quit to leave.")

			done := make(chan struct{})
			go readChatMessages(conn, done)

			interrupts := make(chan os.Signal, 1)
			signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(interrupts)

			input := make(chan string)
			go scanChatInput(input)

			for {
				select {
				case <-done:
					return nil
				case <-interrupts:
					_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return nil
				case line, ok := <-input:
					if !ok {
						_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
						return nil
					}
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if strings.EqualFold(line, "/quit") {
						_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
						return nil
					}

					msg := models.ChatMessage{Message: line}
					if err := conn.WriteJSON(msg); err != nil {
						return fmt.Errorf("send message: %w", err)
					}
				}
			}
		},
	}

	cmd.Flags().StringVar(&mangaID, "manga-id", "", "join a manga-specific room instead of general chat")
	cmd.Flags().StringVar(&name, "name", "", "guest display name")
	cmd.Flags().StringVar(&token, "token", "", "JWT token for registered-user chat")
	cmd.Flags().BoolVar(&guest, "guest", false, "join as an unregistered guest")
	return cmd
}

func buildWebSocketURL(apiBase, mangaID, name string, guest bool) (string, error) {
	base, err := url.Parse(strings.TrimRight(apiBase, "/"))
	if err != nil {
		return "", fmt.Errorf("parse API URL: %w", err)
	}

	switch base.Scheme {
	case "https":
		base.Scheme = "wss"
	default:
		base.Scheme = "ws"
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/ws"

	q := base.Query()
	if strings.TrimSpace(mangaID) != "" {
		q.Set("manga_id", strings.TrimSpace(mangaID))
	}
	if guest {
		q.Set("guest", strings.TrimSpace(name))
	}
	base.RawQuery = q.Encode()

	return base.String(), nil
}

func readChatMessages(conn *websocket.Conn, done chan<- struct{}) {
	defer close(done)

	for {
		var msg models.ChatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				fmt.Fprintf(os.Stderr, "chat closed: %v\n", err)
			}
			return
		}

		label := msg.Username
		if label == "" {
			label = msg.UserID
		}
		when := time.UnixMilli(msg.Timestamp).Format("15:04:05")
		fmt.Printf("[%s] %s: %s\n", when, label, msg.Message)
	}
}

func scanChatInput(out chan<- string) {
	defer close(out)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		out <- scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
	}
}

func defaultGuestName() string {
	if env := strings.TrimSpace(os.Getenv("USERNAME")); env != "" {
		return env
	}
	if env := strings.TrimSpace(os.Getenv("USER")); env != "" {
		return env
	}
	return "guest"
}
