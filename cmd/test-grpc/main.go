package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	grpcserver "github.com/plbth/mangahub/internal/grpc"
	"github.com/plbth/mangahub/pkg/database"
	"github.com/plbth/mangahub/pkg/models"
	"github.com/plbth/mangahub/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var (
		useRunning = flag.Bool("use-running", false, "connect to an already-running gRPC server instead of starting a test server")
		addr       = flag.String("addr", "localhost:9092", "gRPC server address")
		port       = flag.Int("port", 9092, "port for the self-contained test server")
		mangaID    = flag.String("manga-id", "one-piece", "manga id to fetch when using an already-running server")
		query      = flag.String("query", "one", "search query")
	)
	flag.Parse()

	if *useRunning {
		runClientChecks(*addr, *mangaID, *query)
		return
	}

	// 1. Initialize database
	db, err := database.InitDB(":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	repo := database.NewSQLiteRepository(db)

	// 2. Add test data
	manga := &models.Manga{
		ID:            "test-manga",
		Title:         "Test Manga",
		Author:        "Test Author",
		Genres:        []string{"action"},
		Status:        "ongoing",
		TotalChapters: 100,
		Description:   "Test description",
	}
	if err := repo.AddManga(manga); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Test manga added")

	// 3. Start gRPC server
	server := grpcserver.NewMangaServer(repo)
	grpcSrv, err := server.Start(*port)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Shutdown(grpcSrv, context.Background())

	time.Sleep(100 * time.Millisecond) // Give server time to start
	fmt.Printf("✓ gRPC server started on :%d\n", *port)

	runClientChecks(fmt.Sprintf("localhost:%d", *port), "test-manga", "test")
}

func runClientChecks(addr, mangaID, query string) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := proto.NewMangaServiceClient(conn)
	fmt.Printf("✓ gRPC client connected to %s\n", addr)

	resp, err := client.GetManga(context.Background(), &proto.GetMangaRequest{Id: mangaID})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✓ GetManga RPC succeeded: %s by %s\n", resp.Title, resp.Author)

	searchResp, err := client.SearchManga(context.Background(), &proto.SearchRequest{Query: query})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✓ SearchManga RPC succeeded: found %d results\n", len(searchResp.Results))

	fmt.Println("\n✅ All gRPC tests passed!")
}
