package udp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/plbth/mangahub/pkg/models"
)

// Server is a simple UDP notification server.
// It keeps a registry of subscribed client addresses and can broadcast
// notifications to all of them using WriteToUDP.
type Server struct {
	Addr string

	mu      sync.RWMutex
	clients map[string]*net.UDPAddr
	conn    *net.UDPConn
	logger  *log.Logger
}

// NewServer creates a UDP server bound to the provided address.
// Example address: ":9091" or "localhost:9091".
func NewServer(addr string) *Server {
	return &Server{
		Addr:    addr,
		clients: make(map[string]*net.UDPAddr),
		logger:   log.Default(),
	}
}

// SetLogger allows the caller to inject a custom logger.
func (s *Server) SetLogger(l *log.Logger) {
	if l != nil {
		s.logger = l
	}
}

// Start listens for UDP packets and handles subscription / broadcast commands.
// It blocks until the context is canceled or a fatal listen error occurs.
func (s *Server) Start(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", s.Addr)
	if err != nil {
		return fmt.Errorf("resolve udp addr: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	s.conn = conn

	s.logger.Printf("UDP server listening on %s", conn.LocalAddr().String())

	done := make(chan error, 1)

	go func() {
		done <- s.serve()
	}()

	select {
	case <-ctx.Done():
		_ = s.Close()
		return ctx.Err()
	case err := <-done:
		_ = s.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
		return nil
	}
}

// Close closes the UDP socket.
func (s *Server) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

// ClientCount returns the number of registered clients.
func (s *Server) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

func (s *Server) serve() error {
	buf := make([]byte, 4096)

	for {
		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}

		payload := strings.TrimSpace(string(buf[:n]))
		if payload == "" {
			continue
		}

		if err := s.handlePacket(payload, remoteAddr); err != nil {
			s.logger.Printf("udp packet error from %s: %v", remoteAddr.String(), err)
		}
	}
}

func (s *Server) handlePacket(payload string, remoteAddr *net.UDPAddr) error {
	upper := strings.ToUpper(strings.TrimSpace(payload))

	switch {
	case upper == "SUBSCRIBE" || strings.HasPrefix(upper, "SUBSCRIBE "):
		return s.registerClient(remoteAddr)

	case upper == "UNSUBSCRIBE" || strings.HasPrefix(upper, "UNSUBSCRIBE "):
		s.unregisterClient(remoteAddr)
		return s.reply(remoteAddr, "OK UNSUBSCRIBED")

	case upper == "LIST":
		return s.reply(remoteAddr, fmt.Sprintf("OK CLIENTS %d", s.ClientCount()))

	case upper == "PING":
		return s.reply(remoteAddr, "PONG")

	case strings.HasPrefix(upper, "PUBLISH "):
		message := strings.TrimSpace(payload[len("PUBLISH "):])
		if message == "" {
			return s.reply(remoteAddr, "ERR empty message")
		}
		s.BroadcastRaw(message)
		return s.reply(remoteAddr, "OK BROADCASTED")

	case strings.HasPrefix(upper, "NOTIFY "):
		// Convenience command:
		// NOTIFY <manga_id>|<message>
		body := strings.TrimSpace(payload[len("NOTIFY "):])
		parts := strings.SplitN(body, "|", 2)
		if len(parts) != 2 {
			return s.reply(remoteAddr, "ERR usage: NOTIFY <manga_id>|<message>")
		}
		notif := models.Notification{
			Type:      "new_chapter",
			MangaID:   strings.TrimSpace(parts[0]),
			Message:   strings.TrimSpace(parts[1]),
			Timestamp: time.Now().Unix(),
		}
		return s.NotifyAll(notif)

	default:
		// By default, treat a message from an unknown client as a subscription
		// request first. This makes testing with netcat straightforward:
		//   echo -n "test" | nc -u -w1 localhost 9091
		//
		// If the client is already registered, we broadcast the raw payload to
		// all clients for quick smoke testing.
		if s.isRegistered(remoteAddr) {
			return s.BroadcastRaw(payload)
		}
		return s.registerClient(remoteAddr)
	}
}

func (s *Server) registerClient(remoteAddr *net.UDPAddr) error {
	s.mu.Lock()
	s.clients[remoteAddr.String()] = remoteAddr
	s.mu.Unlock()

	return s.reply(remoteAddr, "OK SUBSCRIBED")
}

func (s *Server) unregisterClient(remoteAddr *net.UDPAddr) {
	s.mu.Lock()
	delete(s.clients, remoteAddr.String())
	s.mu.Unlock()
}

func (s *Server) isRegistered(remoteAddr *net.UDPAddr) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.clients[remoteAddr.String()]
	return ok
}

func (s *Server) reply(remoteAddr *net.UDPAddr, message string) error {
	if s.conn == nil {
		return errors.New("udp server is not running")
	}
	_, err := s.conn.WriteToUDP([]byte(message), remoteAddr)
	return err
}

// BroadcastRaw sends a plain text message to every subscribed client.
// It removes clients that are no longer reachable.
func (s *Server) BroadcastRaw(message string) error {
	s.mu.RLock()
	addrs := make([]*net.UDPAddr, 0, len(s.clients))
	for _, addr := range s.clients {
		addrs = append(addrs, addr)
	}
	s.mu.RUnlock()

	var failed []string
	for _, addr := range addrs {
		if _, err := s.conn.WriteToUDP([]byte(message), addr); err != nil {
			failed = append(failed, addr.String())
		}
	}

	if len(failed) > 0 {
		s.mu.Lock()
		for _, key := range failed {
			delete(s.clients, key)
		}
		s.mu.Unlock()
		return fmt.Errorf("broadcast failed for %d client(s)", len(failed))
	}
	return nil
}

// NotifyAll broadcasts a structured notification to all subscribed clients.
// The payload is encoded as JSON so it can be parsed by future clients.
func (s *Server) NotifyAll(notification models.Notification) error {
	if notification.Timestamp == 0 {
		notification.Timestamp = time.Now().Unix()
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	return s.BroadcastRaw(string(payload))
}

// NotifyChapterRelease is a helper for the common "new chapter" event.
func (s *Server) NotifyChapterRelease(mangaID, message string) error {
	return s.NotifyAll(models.Notification{
		Type:      "new_chapter",
		MangaID:   mangaID,
		Message:   message,
		Timestamp: time.Now().Unix(),
	})
}

// SnapshotClients returns a copy of the current client registry.
// Useful for debugging and tests.
func (s *Server) SnapshotClients() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]string, 0, len(s.clients))
	for k := range s.clients {
		out = append(out, k)
	}
	return out
}
