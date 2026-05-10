package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/plbth/mangahub/pkg/models"
)

// ── Tuning constants ──────────────────────────────────────────────────────────

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer (bytes).
	maxMessageSize = 2048
)

// ── Upgrader ──────────────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// CheckOrigin controls CORS for the WebSocket handshake.
	// For production, replace this with a real origin whitelist.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client is the bridge between a single WebSocket connection and the ChatHub.
//
// Gorilla concurrency rule: each websocket.Conn supports at most one
// concurrent reader and one concurrent writer.  We honour this with two
// dedicated goroutines: readPump (reader) and writePump (writer).
// Neither pump ever calls the other's method on the connection.
type Client struct {
	hub      *ChatHub
	conn     *websocket.Conn

	// send is a buffered channel of outbound payloads owned exclusively by
	// writePump. fanOut enqueues here; it never writes to conn directly.
	send chan []byte

	// Identifying fields set once at connection time and read-only thereafter.
	userID   string
	username string
	mangaID  string // "" → general chat
}

// ── ServeWS ───────────────────────────────────────────────────────────────────

// ServeWS upgrades an HTTP request to a WebSocket connection, creates a Client,
// registers it with the hub, and starts its read/write pumps.
//
// Expected query parameters:
//
//	user_id   – authenticated user's ID   (validated upstream by JWT middleware)
//	username  – display name
//	manga_id  – (optional) target manga room; omit for general chat
//
// In a real deployment the JWT middleware stores userID/username in the
// request context; read from r.Context() before falling back to query params.
func ServeWS(w http.ResponseWriter, r *http.Request, hub *ChatHub) {
	ctx := r.Context()
	userID, _ := ctx.Value("userID").(string)
	username, _ := ctx.Value("username").(string)

	// Fallback to query string if auth context is unavailable.
	if userID == "" || username == "" {
		q := r.URL.Query()
		if userID == "" {
			userID = q.Get("user_id")
		}
		if username == "" {
			username = q.Get("username")
		}
	}

	mangaID := r.URL.Query().Get("manga_id") // empty string → general chat

	if userID == "" || username == "" {
		http.Error(w, "user_id and username are required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrader already wrote an HTTP error response on failure.
		log.Printf("[WS] Upgrade error for user %s: %v", userID, err)
		return
	}

	client := &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 256),
		userID:   userID,
		username: username,
		mangaID:  mangaID,
	}

	// Register with the hub before starting pumps so we never miss a message.
	hub.register <- client

	// Each pump runs in its own goroutine per Gorilla's documented pattern.
	go client.writePump()
	go client.readPump()
}

// ── readPump ──────────────────────────────────────────────────────────────────

// readPump is the sole goroutine that reads from conn.
//
// It decodes incoming JSON ChatMessage payloads and forwards them to the hub's
// broadcast channel.  When the connection closes (EOF, error, or pong timeout)
// it unregisters the client and returns, which lets writePump drain and exit.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)

	// Establish the initial read deadline; reset on every pong received.
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, rawMsg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
				websocket.CloseNormalClosure,
			) {
				log.Printf("[WS] Unexpected close from %s (%s): %v",
					c.userID, c.username, err)
			}
			return // triggers deferred unregister
		}

		var msg models.ChatMessage
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			log.Printf("[WS] Malformed JSON from %s: %v", c.userID, err)
			continue // skip bad payloads, keep connection alive
		}

		// Authoritative fields come from the server, not the client payload,
		// so we overwrite whatever the client sent.
		msg.UserID = c.userID
		msg.Username = c.username
		msg.MangaID = c.mangaID
		msg.Timestamp = time.Now().UnixMilli()

		// Non-blocking enqueue: if the broadcast channel is full we log and
		// drop rather than blocking the reader goroutine.
		select {
		case c.hub.broadcast <- &msg:
		default:
			log.Printf("[WS] Broadcast channel full – dropping message from %s", c.userID)
		}
	}
}

// ── writePump ─────────────────────────────────────────────────────────────────

// writePump is the sole goroutine that writes to conn.
//
// It drains the client's send channel and writes each payload to the WebSocket.
// A ticker fires periodic ping frames to keep the connection alive and allow
// the peer's pong handler (in readPump) to reset its read deadline.
// When the hub closes the send channel (on unregister), writePump sends a
// Close frame and exits.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {

		// ── Outbound message ──────────────────────────────────────────────────
		case payload, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// Hub closed the channel: send a Close frame and exit.
				_ = c.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Printf("[WS] NextWriter error for %s: %v", c.userID, err)
				return
			}

			if _, err = w.Write(payload); err != nil {
				log.Printf("[WS] Write error for %s: %v", c.userID, err)
				return
			}

			// Coalesce any messages that arrived while we held the writer.
			// This batches buffered payloads into a single WebSocket frame,
			// reducing syscall overhead under burst load.
		coalesce:
			for {
				select {
				case next, ok := <-c.send:
					if !ok {
						break coalesce
					}
					if _, err = w.Write(next); err != nil {
						log.Printf("[WS] Write error (coalesce) for %s: %v", c.userID, err)
						_ = w.Close()
						return
					}
				default:
					break coalesce
				}
			}

			if err = w.Close(); err != nil {
				log.Printf("[WS] Writer close error for %s: %v", c.userID, err)
				return
			}

		// ── Keepalive ping ────────────────────────────────────────────────────
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[WS] Ping error for %s: %v – closing", c.userID, err)
				return
			}
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// marshalMessage JSON-encodes a ChatMessage and returns the byte slice.
// Returns nil and logs on error (should never happen with a well-formed struct).
func marshalMessage(msg *models.ChatMessage) []byte {
	b, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] Marshal error: %v", err)
		return nil
	}
	return b
}