package tcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/plbth/mangahub/pkg/models"
)

// broadcastJob bundles an update with the sender's ID so we can skip them.
type broadcastJob struct {
	senderID string
	update   models.ProgressUpdate
}

// TCPServer manages a pool of TCP connections and fans out ProgressUpdate
// payloads to every connected client except the one that sent them.
type TCPServer struct {
	port     int
	listener net.Listener

	// Connection pool – keyed by remote address string.
	mu    sync.RWMutex
	conns map[string]net.Conn

	// A single broadcaster goroutine drains this channel.
	broadcast chan broadcastJob

	// Closed by Shutdown() to stop the accept-loop.
	quit chan struct{}
	wg   sync.WaitGroup
}

// NewTCPServer creates a TCPServer that will listen on the given port.
// Call Start() to begin accepting connections.
func NewTCPServer(port int) *TCPServer {
	return &TCPServer{
		port:      port,
		conns:     make(map[string]net.Conn),
		broadcast: make(chan broadcastJob, 256), // buffered – don't block readers
		quit:      make(chan struct{}),
	}
}

// Start opens the listener, launches the broadcaster goroutine, and begins
// accepting connections.  It is non-blocking: the accept-loop runs in its own
// goroutine.  Returns an error if the port cannot be bound.
func (s *TCPServer) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("tcp: failed to listen on port %d: %w", s.port, err)
	}
	s.listener = ln
	log.Printf("[TCP] Server listening on :%d", s.port)

	// Broadcaster goroutine – one writer to all conns.
	s.wg.Add(1)
	go s.runBroadcaster()

	// Accept loop goroutine.
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Shutdown gracefully stops the server.  It closes the listener (unblocking
// Accept), signals internal goroutines, closes every open connection, and
// waits for all goroutines to exit.
func (s *TCPServer) Shutdown(ctx context.Context) error {
	// Signal goroutines first so listener/connection close errors are treated
	// as expected shutdown noise.
	close(s.quit)

	// Signal the accept-loop to stop by closing the listener.
	if s.listener != nil {
		s.listener.Close()
	}

	// Close every connected client so their read-loops unblock.
	s.mu.Lock()
	for id, conn := range s.conns {
		conn.Close()
		delete(s.conns, id)
	}
	s.mu.Unlock()

	// Wait for all goroutines, or honour the caller's deadline.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("[TCP] Server shut down cleanly.")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("tcp: shutdown timed out: %w", ctx.Err())
	}
}

// ── internal loops ────────────────────────────────────────────────────────────

// acceptLoop runs until the listener is closed.
func (s *TCPServer) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// A "use of closed network connection" error is expected on shutdown.
			select {
			case <-s.quit:
				return // clean shutdown
			default:
				log.Printf("[TCP] Accept error: %v", err)
				return
			}
		}

		s.addConn(conn)
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

// runBroadcaster serialises writes to all connections so individual
// handleConn goroutines never race on a shared net.Conn.
func (s *TCPServer) runBroadcaster() {
	defer s.wg.Done()

	for {
		select {
		case job, ok := <-s.broadcast:
			if !ok {
				return
			}
			s.fanOut(job)
		case <-s.quit:
			// Drain any remaining jobs before exiting.
			for {
				select {
				case job := <-s.broadcast:
					s.fanOut(job)
				default:
					return
				}
			}
		}
	}
}

// handleConn reads newline-delimited JSON ProgressUpdate payloads from a
// single connection and forwards each one to the broadcast channel.
func (s *TCPServer) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.removeConn(conn)
		conn.Close()
	}()

	id := conn.RemoteAddr().String()
	log.Printf("[TCP] Client connected: %s", id)

	decoder := json.NewDecoder(conn)
	for {
		var update models.ProgressUpdate
		if err := decoder.Decode(&update); err != nil {
			if err == io.EOF {
				log.Printf("[TCP] Client disconnected: %s", id)
			} else {
				// Swallow errors that arise from a connection closed during shutdown.
				select {
				case <-s.quit:
				default:
					log.Printf("[TCP] Read error from %s: %v", id, err)
				}
			}
			return
		}
		update.Timestamp = time.Now().UnixMilli()

		log.Printf("[TCP] Received update from %s – user=%s manga=%s chapter=%d",
			id, update.UserID, update.MangaID, update.Chapter)

		// Non-blocking send: if the channel is full we log and skip rather than
		// stall the reading goroutine (back-pressure protection).
		select {
		case s.broadcast <- broadcastJob{senderID: id, update: update}:
		default:
			log.Printf("[TCP] Broadcast channel full; dropping update from %s", id)
		}
	}
}

// fanOut writes the serialised update to every connection except the sender.
// Connections that fail to write are removed from the pool.
func (s *TCPServer) fanOut(job broadcastJob) {
	payload, err := json.Marshal(job.update)
	if err != nil {
		log.Printf("[TCP] Marshal error: %v", err)
		return
	}
	payload = append(payload, '\n') // newline delimiter for the remote decoder

	s.mu.RLock()
	targets := make(map[string]net.Conn, len(s.conns))
	for id, conn := range s.conns {
		targets[id] = conn
	}
	s.mu.RUnlock()

	var failed []string
	for id, conn := range targets {
		if id == job.senderID {
			continue // don't echo back to the sender
		}
		if _, err := conn.Write(payload); err != nil {
			log.Printf("[TCP] Write error to %s: %v – removing client", id, err)
			failed = append(failed, id)
		}
	}

	// Evict dead connections outside the read-lock.
	if len(failed) > 0 {
		s.mu.Lock()
		for _, id := range failed {
			if c, ok := s.conns[id]; ok {
				c.Close()
				delete(s.conns, id)
			}
		}
		s.mu.Unlock()
	}
}

// ── pool helpers ──────────────────────────────────────────────────────────────

func (s *TCPServer) addConn(conn net.Conn) {
	s.mu.Lock()
	s.conns[conn.RemoteAddr().String()] = conn
	s.mu.Unlock()
	log.Printf("[TCP] Pool size: %d", s.poolSize())
}

func (s *TCPServer) removeConn(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn.RemoteAddr().String())
	s.mu.Unlock()
	log.Printf("[TCP] Pool size: %d", s.poolSize())
}

func (s *TCPServer) poolSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.conns)
}