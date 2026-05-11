package websocket

import (
	"log"
	"sync"

	"github.com/plbth/mangahub/pkg/models"
)

// room is an alias for the set of clients inside one chat room.
// The map value is unused; we use the map purely as a hash-set.
type room map[*Client]bool

// ChatHub is the single source of truth for all WebSocket state.
// ALL mutations to the rooms map happen inside the Run() goroutine –
// this is the canonical Gorilla pattern that avoids any map data race.
type ChatHub struct {
	// rooms[""] holds the general-chat clients.
	// rooms["one-piece"] holds clients in the One Piece manga room, etc.
	rooms map[string]room

	// Inbound messages from any client's readPump.
	broadcast chan *models.ChatMessage

	// register / unregister are sent by ServeWS and writePump respectively.
	register   chan *Client
	unregister chan *Client

	quit      chan struct{}
	closeOnce sync.Once
}

// NewChatHub constructs a ready-to-use hub. Call hub.Run() in a goroutine.
func NewChatHub() *ChatHub {
	return &ChatHub{
		rooms:      make(map[string]room),
		broadcast:  make(chan *models.ChatMessage, 256),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		quit:       make(chan struct{}),
	}
}

// Close signals the hub's Run loop to shut down.
func (h *ChatHub) Close() {
	h.closeOnce.Do(func() {
		close(h.quit)
	})
}

// Run is the hub's event loop. It must run in exactly one goroutine.
// It owns all reads and writes to the rooms map.
func (h *ChatHub) Run() {
	for {
		select {

		// ── Registration ──────────────────────────────────────────────────────
		case client := <-h.register:
			roomKey := client.mangaID // "" for general chat

			if h.rooms[roomKey] == nil {
				h.rooms[roomKey] = make(room)
			}
			h.rooms[roomKey][client] = true

			log.Printf("[WS] Client %s (%s) joined room %q  (room size: %d)",
				client.userID, client.username, roomLabel(roomKey), len(h.rooms[roomKey]))

		// ── Unregistration ────────────────────────────────────────────────────
		case client := <-h.unregister:
			roomKey := client.mangaID

			if r, ok := h.rooms[roomKey]; ok {
				if _, exists := r[client]; exists {
					delete(r, client)
					// Drain the send channel so the writePump can exit cleanly.
					close(client.send)

					log.Printf("[WS] Client %s (%s) left room %q  (room size: %d)",
						client.userID, client.username, roomLabel(roomKey), len(r))

					// Garbage-collect empty rooms (except general chat).
					if len(r) == 0 && roomKey != "" {
						delete(h.rooms, roomKey)
						log.Printf("[WS] Room %q removed (empty)", roomKey)
					}
				}
			}

		// ── Broadcast ─────────────────────────────────────────────────────────
		case msg := <-h.broadcast:
			h.fanOut(msg)

		case <-h.quit:
			h.closeAllClients()
			return
		}
	}
}

// closeAllClients closes all client send channels and clears the rooms map.
// This must only be called from the Run goroutine.
func (h *ChatHub) closeAllClients() {
	for roomKey, r := range h.rooms {
		for client := range r {
			close(client.send)
			delete(r, client)
		}
		if len(r) == 0 {
			delete(h.rooms, roomKey)
		}
	}
}

// fanOut delivers msg to every client in the target room, including the sender.
// Dead clients (send channel full) are evicted immediately.
func (h *ChatHub) fanOut(msg *models.ChatMessage) {
	roomKey := msg.MangaID // "" → general chat

	r, ok := h.rooms[roomKey]
	if !ok {
		return // nobody in this room
	}

	log.Printf("[WS] Message from %s (%s) in room %q: %s",
		msg.UserID, msg.Username, roomLabel(roomKey), msg.Message)

	payload := marshalMessage(msg)
	if payload == nil {
		return
	}

	var evict []*Client
	for client := range r {
		select {
		case client.send <- payload:
			// delivered to writePump buffer
		default:
			// The client's send channel is full – treat it as dead.
			log.Printf("[WS] Send buffer full for client %s – evicting", client.userID)
			evict = append(evict, client)
		}
	}

	// Evict dead clients while we still own the map (same goroutine).
	for _, client := range evict {
		delete(r, client)
		close(client.send)
	}
	if len(r) == 0 && roomKey != "" {
		delete(h.rooms, roomKey)
	}
}

// roomLabel returns a human-readable room name for logging.
func roomLabel(key string) string {
	if key == "" {
		return "general"
	}
	return key
}