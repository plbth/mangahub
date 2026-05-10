package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/plbth/mangahub/pkg/database"
    "github.com/plbth/mangahub/pkg/models"
    grpcserver "github.com/plbth/mangahub/internal/grpc"
    "github.com/plbth/mangahub/proto"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
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
    grpcSrv, err := server.Start(9092)
    if err != nil {
        log.Fatal(err)
    }
    defer server.Shutdown(grpcSrv, context.Background())

    time.Sleep(100 * time.Millisecond) // Give server time to start
    fmt.Println("✓ gRPC server started on :9092")

    // 4. Connect as client
    conn, err := grpc.Dial("localhost:9092", grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := proto.NewMangaServiceClient(conn)
    fmt.Println("✓ gRPC client connected")

    // 5. Test GetManga RPC
    resp, err := client.GetManga(context.Background(), &proto.GetMangaRequest{Id: "test-manga"})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("✓ GetManga RPC succeeded: %s by %s\n", resp.Title, resp.Author)

    // 6. Test SearchManga RPC
    searchResp, err := client.SearchManga(context.Background(), &proto.SearchRequest{Query: "test"})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("✓ SearchManga RPC succeeded: found %d results\n", len(searchResp.Results))

    fmt.Println("\n✅ All gRPC tests passed!")
}