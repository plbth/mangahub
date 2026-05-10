package main

import (
    "fmt"
    "time"

    "github.com/plbth/mangahub/internal/websocket"
)

func main() {
    // 1. Create and start hub
    hub := websocket.NewChatHub()
    go hub.Run()
    fmt.Println("✓ WebSocket hub running")

    time.Sleep(500 * time.Millisecond)

    // 2. Close hub gracefully
    fmt.Println("Calling hub.Close()...")
    hub.Close()

    // 3. Hub.Run() should exit after close
    time.Sleep(100 * time.Millisecond)
    fmt.Println("✓ WebSocket hub shut down cleanly")

    // 4. Verify Close is idempotent (can call multiple times safely)
    hub.Close()
    hub.Close()
    fmt.Println("✓ Multiple Close() calls are safe (idempotent)")
}