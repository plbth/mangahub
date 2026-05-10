package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	grpcserver "github.com/plbth/mangahub/internal/grpc"
	"github.com/plbth/mangahub/internal/tcp"
	"github.com/plbth/mangahub/internal/websocket"
	"github.com/plbth/mangahub/pkg/database"
	gogrpc "google.golang.org/grpc"
)

func main() {
    log.Println("[ORCHESTRATOR] MangaHub Server Orchestrator starting...")

    if err := os.MkdirAll("data", 0755); err != nil {
        log.Fatalf("[ORCHESTRATOR] Failed to create data directory: %v", err)
    }

    db, err := database.InitDB("data/mangahub.db")
    if err != nil {
        log.Fatalf("[ORCHESTRATOR] Failed to initialize database: %v", err)
    }
    log.Println("[ORCHESTRATOR] Database initialized")
    defer func() {
        if closeErr := db.Close(); closeErr != nil {
            log.Printf("[ORCHESTRATOR] Database close error: %v", closeErr)
        }
    }()

    repo := database.NewSQLiteRepository(db)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    var wg sync.WaitGroup

    // Signal handling.
    sigChan := make(chan os.Signal, 1)
    shutdownSignal := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    go func() {
        shutdownSignal <- <-sigChan
    }()

    // Start TCP server.
    tcpServer := tcp.NewTCPServer(9090)
    tcpStarted := make(chan struct{}, 1)
    startErrors := make(chan error, 2)

    wg.Add(1)
    go func() {
        defer wg.Done()
        log.Println("[ORCHESTRATOR] Starting TCP server on :9090")
        if err := tcpServer.Start(); err != nil {
            startErrors <- fmt.Errorf("tcp start failed: %w", err)
            return
        }
        tcpStarted <- struct{}{}
        <-ctx.Done()
    }()

    // Start WebSocket hub.
    hub := websocket.NewChatHub()
    log.Println("[ORCHESTRATOR] Starting WebSocket hub (served via HTTP on :9093)")
    wg.Add(1)
    go func() {
        defer wg.Done()
        hub.Run()
    }()

    // Start gRPC server.
    grpcServer := grpcserver.NewMangaServer(repo)
    grpcStarted := make(chan *gogrpc.Server, 1)

    wg.Add(1)
    go func() {
        defer wg.Done()
        log.Println("[ORCHESTRATOR] Starting gRPC server on :9092")
        srv, err := grpcServer.Start(9092)
        if err != nil {
            startErrors <- fmt.Errorf("grpc start failed: %w", err)
            return
        }
        grpcStarted <- srv
        <-ctx.Done()
    }()

    // Track gRPC server instance for shutdown.
    var grpcSrv *gogrpc.Server

    // Wait for TCP and gRPC to start (or fail).
    startedTCP := false
    startedGRPC := false
    startupTimer := time.NewTimer(10 * time.Second)
    for !(startedTCP && startedGRPC) {
        select {
        case err := <-startErrors:
            log.Fatalf("[ORCHESTRATOR] Startup error: %v", err)
        case <-tcpStarted:
            startedTCP = true
        case srv := <-grpcStarted:
            grpcSrv = srv
            startedGRPC = true
        case <-startupTimer.C:
            log.Fatal("[ORCHESTRATOR] Startup timeout waiting for servers to signal readiness")
        }
    }
    startupTimer.Stop()

    log.Println("[ORCHESTRATOR] HTTP API server pending (Person B) on :8080")
    log.Println("[ORCHESTRATOR] UDP notification server pending (Person B) on :9091")
    log.Println("[ORCHESTRATOR] All servers started successfully")

    // Block until shutdown signal.
    sig := <-shutdownSignal
    log.Printf("[ORCHESTRATOR] Received shutdown signal: %s", sig.String())

    cancel()

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer shutdownCancel()

    var shutdownErrors bool
    var shutdownMu sync.Mutex
    var shutdownWg sync.WaitGroup

    shutdownWg.Add(1)
    go func() {
        defer shutdownWg.Done()
        log.Println("[ORCHESTRATOR] Shutting down TCP server...")
        if err := tcpServer.Shutdown(shutdownCtx); err != nil {
            shutdownMu.Lock()
            shutdownErrors = true
            shutdownMu.Unlock()
            log.Printf("[ORCHESTRATOR] TCP shutdown error: %v", err)
        }
    }()

    shutdownWg.Add(1)
    go func() {
        defer shutdownWg.Done()
        log.Println("[ORCHESTRATOR] Shutting down gRPC server...")
        if err := grpcServer.Shutdown(grpcSrv, shutdownCtx); err != nil {
            shutdownMu.Lock()
            shutdownErrors = true
            shutdownMu.Unlock()
            log.Printf("[ORCHESTRATOR] gRPC shutdown error: %v", err)
        }
    }()

    log.Println("[ORCHESTRATOR] Shutting down WebSocket hub...")
    hub.Close()

    shutdownWg.Wait()

    // Wait for orchestrator goroutines to exit.
    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()

    select {
    case <-done:
    case <-time.After(5 * time.Second):
        shutdownMu.Lock()
        shutdownErrors = true
        shutdownMu.Unlock()
        log.Println("[ORCHESTRATOR] Timeout waiting for goroutines to exit")
    }

    shutdownMu.Lock()
    hadShutdownErrors := shutdownErrors
    shutdownMu.Unlock()

    if hadShutdownErrors {
        log.Println("[ORCHESTRATOR] Some servers did not shut down gracefully")
        os.Exit(1)
    }

    log.Println("[ORCHESTRATOR] All servers shut down cleanly")
}
