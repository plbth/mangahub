package main

import (
	"flag"
	"log"
	"os"

	httpapi "github.com/plbth/mangahub/internal/http"
	"github.com/plbth/mangahub/internal/websocket"
	"github.com/plbth/mangahub/pkg/database"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var (
		addr      = flag.String("addr", ":8080", "HTTP listen address")
		jwtSecret = flag.String("jwt-secret", "mangahub-dev-secret", "JWT signing secret")
	)
	flag.Parse()

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	db, err := database.InitDB("data/mangahub.db")
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("close db: %v", closeErr)
		}
	}()

	repo := database.NewSQLiteRepository(db)
	hub := websocket.NewChatHub()
	go hub.Run()

	grpcConn, err := grpc.Dial("127.0.0.1:9092", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("connect gRPC: %v", err)
	}
	defer func() {
		if closeErr := grpcConn.Close(); closeErr != nil {
			log.Printf("close gRPC: %v", closeErr)
		}
	}()

	srv, err := httpapi.NewServer(httpapi.Config{
		JWTSecret: *jwtSecret,
	}, repo, hub, grpcConn)
	if err != nil {
		log.Fatalf("create HTTP server: %v", err)
	}

	log.Printf("MangaHub HTTP API listening on %s", *addr)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("run HTTP server: %v", err)
	}
}
