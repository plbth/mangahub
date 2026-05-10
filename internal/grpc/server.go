package grpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/plbth/mangahub/pkg/database"
	"github.com/plbth/mangahub/pkg/models"
	"github.com/plbth/mangahub/proto"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MangaServer implements the internal gRPC MangaService.
type MangaServer struct {
    proto.UnimplementedMangaServiceServer
    repo database.Repository
}

// NewMangaServer constructs a MangaServer with the given repository.
func NewMangaServer(repo database.Repository) *MangaServer {
    return &MangaServer{repo: repo}
}

// GetManga retrieves a manga by ID.
func (s *MangaServer) GetManga(ctx context.Context, req *proto.GetMangaRequest) (*proto.MangaResponse, error) {
    if req == nil {
        return nil, status.Error(codes.InvalidArgument, "request is required")
    }

    manga, err := s.repo.GetManga(req.Id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, status.Error(codes.NotFound, "manga not found")
        }
        wrapped := fmt.Errorf("grpc: get manga failed: %w", err)
        log.Printf("[GRPC] GetManga error: %v", wrapped)
        return nil, status.Error(codes.Internal, "failed to get manga")
    }

    return &proto.MangaResponse{
        Id:            manga.ID,
        Title:         manga.Title,
        Author:        manga.Author,
        Genres:        manga.Genres,
        Status:        manga.Status,
        TotalChapters: int32(manga.TotalChapters),
        Description:   manga.Description,
        CoverUrl:      manga.CoverURL,
    }, nil
}

// SearchManga searches for manga by query and optional genre filter.
func (s *MangaServer) SearchManga(ctx context.Context, req *proto.SearchRequest) (*proto.SearchResponse, error) {
    if req == nil {
        return nil, status.Error(codes.InvalidArgument, "request is required")
    }

    results, err := s.repo.SearchManga(req.Query, req.Genre)
    if err != nil {
        wrapped := fmt.Errorf("grpc: search manga failed: %w", err)
        log.Printf("[GRPC] SearchManga error: %v", wrapped)
        return nil, status.Error(codes.Internal, "failed to search manga")
    }

    response := &proto.SearchResponse{Results: make([]*proto.MangaResponse, 0, len(results))}
    for _, manga := range results {
        response.Results = append(response.Results, &proto.MangaResponse{
            Id:            manga.ID,
            Title:         manga.Title,
            Author:        manga.Author,
            Genres:        manga.Genres,
            Status:        manga.Status,
            TotalChapters: int32(manga.TotalChapters),
            Description:   manga.Description,
            CoverUrl:      manga.CoverURL,
        })
    }

    return response, nil
}

// UpdateProgress updates a user's reading progress.
func (s *MangaServer) UpdateProgress(ctx context.Context, req *proto.UpdateProgressRequest) (*proto.ProgressResponse, error) {
    if req == nil {
        return &proto.ProgressResponse{Success: false, Message: "request is required"}, nil
    }

    progress := &models.UserProgress{
        UserID:         req.UserId,
        MangaID:        req.MangaId,
        CurrentChapter: int(req.Chapter),
        Volume:         int(req.Volume),
        Status:         req.Status,
        Rating:         int(req.Rating),
    }

    if err := s.repo.UpdateProgress(progress); err != nil {
        wrapped := fmt.Errorf("grpc: update progress failed: %w", err)
        log.Printf("[GRPC] UpdateProgress error: %v", wrapped)
        return &proto.ProgressResponse{Success: false, Message: "failed to update progress"}, nil
    }

    return &proto.ProgressResponse{Success: true, Message: "progress updated"}, nil
}

// Start starts the gRPC server on the given port.
func (s *MangaServer) Start(port int) (*gogrpc.Server, error) {
    listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
    if err != nil {
        return nil, fmt.Errorf("grpc: failed to listen: %w", err)
    }

    grpcServer := gogrpc.NewServer()
    proto.RegisterMangaServiceServer(grpcServer, s)

    go func() {
        if serveErr := grpcServer.Serve(listener); serveErr != nil {
            wrapped := fmt.Errorf("grpc: serve failed: %w", serveErr)
            log.Printf("[GRPC] Serve error: %v", wrapped)
        }
    }()

    return grpcServer, nil
}

// Shutdown gracefully stops the gRPC server.
func (s *MangaServer) Shutdown(grpcServer *gogrpc.Server, ctx context.Context) error {
    if grpcServer == nil {
        return nil
    }

    done := make(chan struct{})
    go func() {
        grpcServer.GracefulStop()
        close(done)
    }()

    select {
    case <-ctx.Done():
        grpcServer.Stop()
        return fmt.Errorf("grpc: shutdown canceled: %w", ctx.Err())
    case <-done:
        return nil
    }
}
