package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/plbth/mangahub/internal/external/jikan"
	"github.com/plbth/mangahub/internal/websocket"
	"github.com/plbth/mangahub/pkg/database"
	"github.com/plbth/mangahub/pkg/models"
	"github.com/plbth/mangahub/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config groups the inputs needed by the HTTP API.
type Config struct {
	JWTSecret string
}

// Server bundles the router and dependencies for the HTTP API.
type Server struct {
	router     *gin.Engine
	repo       database.Repository
	hub        *websocket.ChatHub
	grpcClient proto.MangaServiceClient
	jikan      *jikan.Client
	jwtSecret  []byte
	httpServer *http.Server
}

func NewServer(cfg Config, repo database.Repository, hub *websocket.ChatHub, grpcConn *grpc.ClientConn) (*Server, error) {
	if repo == nil {
		return nil, fmt.Errorf("http: repository is required")
	}
	if hub == nil {
		return nil, fmt.Errorf("http: websocket hub is required")
	}
	if grpcConn == nil {
		return nil, fmt.Errorf("http: grpc connection is required")
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "mangahub-dev-secret"
	}

	s := &Server{
		repo:       repo,
		hub:        hub,
		grpcClient: proto.NewMangaServiceClient(grpcConn),
		jikan:      jikan.NewClient(),
		jwtSecret:  []byte(cfg.JWTSecret),
	}
	s.router = s.buildRouter()
	return s, nil
}

func (s *Server) Router() *gin.Engine {
	return s.router
}

func (s *Server) Run(addr string) error {
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), corsMiddleware())

	r.GET("/health", func(c *gin.Context) {
		c.IndentedJSON(http.StatusOK, gin.H{
			"status":    "ok",
			"service":   "mangahub-http",
			"timestamp": time.Now().UTC(),
		})
	})

	r.GET("/ws", s.authMiddleware(), s.handleWebSocket)

	auth := r.Group("/auth")
	{
		auth.POST("/register", s.handleRegister)
		auth.POST("/login", s.handleLogin)
	}

	manga := r.Group("/manga")
	{
		manga.GET("", s.handleListManga)
		manga.GET("/external/jikan/search", s.handleJikanSearch)
		manga.POST("/external/jikan/import", s.handleJikanImport)
		manga.GET("/:id", s.handleGetManga)
	}

	users := r.Group("/users")
	users.Use(s.authMiddleware())
	{
		users.POST("/library", s.handleAddToLibrary)
		users.GET("/library", s.handleGetLibrary)
		users.PUT("/progress", s.handleUpdateProgress)
	}

	return r
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password" binding:"required"`
}

type AddLibraryRequest struct {
	MangaID        string `json:"manga_id" binding:"required"`
	Status         string `json:"status" binding:"required"`
	CurrentChapter int    `json:"current_chapter"`
	Volume         int    `json:"volume"`
	Rating         int    `json:"rating"`
	Notes          string `json:"notes"`
}

type UpdateProgressRequest struct {
	MangaID string `json:"manga_id" binding:"required"`
	Chapter int    `json:"chapter" binding:"required,min=1"`
	Volume  int    `json:"volume"`
	Rating  int    `json:"rating"`
	Notes   string `json:"notes"`
}

type JikanImportRequest struct {
	Title string `json:"title" binding:"required"`
	Limit int    `json:"limit"`
}

func (s *Server) handleRegister(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashed, err := hashPassword(req.Password)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hashed,
	}

	if err := s.repo.CreateUser(user); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			c.IndentedJSON(http.StatusConflict, gin.H{"error": "username or email already exists"})
			return
		}
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	token, err := s.generateToken(user.ID, user.Username)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.IndentedJSON(http.StatusCreated, models.AuthResponse{
		Token: token,
		User:  *sanitizeUser(user),
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Username == "" {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	user, err := s.repo.GetUserByUsername(req.Username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.IndentedJSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to load account"})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		c.IndentedJSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := s.generateToken(user.ID, user.Username)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.IndentedJSON(http.StatusOK, models.AuthResponse{
		Token: token,
		User:  *sanitizeUser(user),
	})
}

func (s *Server) handleListManga(c *gin.Context) {
	query := strings.TrimSpace(c.Query("query"))
	genre := strings.TrimSpace(c.Query("genre"))
	statusFilter := strings.TrimSpace(c.Query("status"))

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	page := 1
	if raw := c.Query("page"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			page = n
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	results, err := s.searchManga(ctx, query, genre)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to search manga"})
		return
	}

	if statusFilter != "" {
		filtered := make([]models.Manga, 0, len(results))
		for _, item := range results {
			if strings.EqualFold(item.Status, statusFilter) {
				filtered = append(filtered, item)
			}
		}
		results = filtered
	}

	if limit > 0 {
		start := (page - 1) * limit
		if start >= len(results) {
			results = []models.Manga{}
		} else {
			end := start + limit
			if end > len(results) {
				end = len(results)
			}
			results = results[start:end]
		}
	}

	c.IndentedJSON(http.StatusOK, gin.H{
		"count":  len(results),
		"manga":  results,
		"query":  query,
		"genre":  genre,
		"status": statusFilter,
	})
}

func (s *Server) handleGetManga(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := s.grpcClient.GetManga(ctx, &proto.GetMangaRequest{Id: c.Param("id")})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			c.IndentedJSON(http.StatusNotFound, gin.H{"error": "manga not found"})
			return
		}
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch manga"})
		return
	}

	c.IndentedJSON(http.StatusOK, mangaFromProto(resp))
}

func (s *Server) handleJikanSearch(c *gin.Context) {
	title := strings.TrimSpace(c.Query("title"))
	if title == "" {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	limit := 5
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	results, err := s.jikan.SearchManga(ctx, title, limit)
	if err != nil {
		c.IndentedJSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.IndentedJSON(http.StatusOK, gin.H{
		"source": "Jikan / MyAnimeList",
		"count":  len(results),
		"manga":  results,
	})
}

func (s *Server) handleJikanImport(c *gin.Context) {
	var req JikanImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 1
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	results, err := s.jikan.SearchManga(ctx, req.Title, limit)
	if err != nil {
		c.IndentedJSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if len(results) == 0 {
		c.IndentedJSON(http.StatusNotFound, gin.H{"error": "no Jikan results found"})
		return
	}

	manga := results[0]
	if existing, err := s.repo.GetManga(manga.ID); err == nil {
		c.IndentedJSON(http.StatusOK, gin.H{
			"message":  "manga already imported",
			"imported": false,
			"source":   "Jikan / MyAnimeList",
			"manga":    existing,
		})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to check imported manga"})
		return
	}

	if err := s.repo.AddManga(&manga); err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to import manga"})
		return
	}

	c.IndentedJSON(http.StatusCreated, gin.H{
		"message":  "manga imported from Jikan / MyAnimeList",
		"imported": true,
		"source":   "Jikan / MyAnimeList",
		"manga":    manga,
	})
}

func (s *Server) handleAddToLibrary(c *gin.Context) {
	userID, _ := c.Get("userID")
	username, _ := c.Get("username")

	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.IndentedJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	var req AddLibraryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manga, err := s.repo.GetManga(req.MangaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.IndentedJSON(http.StatusNotFound, gin.H{"error": "manga not found"})
			return
		}
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to load manga"})
		return
	}

	progress := models.UserProgress{
		UserID:         userIDStr,
		MangaID:        manga.ID,
		CurrentChapter: req.CurrentChapter,
		Volume:         req.Volume,
		Status:         req.Status,
		Rating:         req.Rating,
		Notes:          req.Notes,
		UpdatedAt:      time.Now().UTC(),
	}

	if err := s.repo.UpdateProgress(&progress); err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to update library"})
		return
	}

	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr = userIDStr
	}

	c.IndentedJSON(http.StatusCreated, gin.H{
		"message":  "added to library",
		"user":     usernameStr,
		"manga":    manga.Title,
		"progress": progress,
	})
}

func (s *Server) handleGetLibrary(c *gin.Context) {
	userID, _ := c.Get("userID")
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.IndentedJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	statusFilter := strings.TrimSpace(c.Query("status"))

	library, err := s.repo.GetUserLibrary(userIDStr)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch library"})
		return
	}

	if statusFilter != "" {
		filtered := make([]models.LibraryView, 0, len(library))
		for _, item := range library {
			if strings.EqualFold(item.Progress.Status, statusFilter) {
				filtered = append(filtered, item)
			}
		}
		library = filtered
	}

	c.IndentedJSON(http.StatusOK, gin.H{
		"count":   len(library),
		"library": library,
	})
}

func (s *Server) handleUpdateProgress(c *gin.Context) {
	userID, _ := c.Get("userID")
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.IndentedJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	var req UpdateProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manga, err := s.repo.GetManga(req.MangaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.IndentedJSON(http.StatusNotFound, gin.H{"error": "manga not found"})
			return
		}
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to load manga"})
		return
	}

	if manga.TotalChapters > 0 && req.Chapter > manga.TotalChapters {
		c.IndentedJSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("chapter %d exceeds manga's total chapters (%d)", req.Chapter, manga.TotalChapters),
		})
		return
	}

	progress := models.UserProgress{
		UserID:         userIDStr,
		MangaID:        manga.ID,
		CurrentChapter: req.Chapter,
		Volume:         req.Volume,
		Status:         "reading",
		Rating:         req.Rating,
		Notes:          req.Notes,
		UpdatedAt:      time.Now().UTC(),
	}

	if err := s.repo.UpdateProgress(&progress); err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "failed to update progress"})
		return
	}

	c.IndentedJSON(http.StatusOK, gin.H{
		"message":  "progress updated successfully",
		"manga":    manga.Title,
		"progress": progress,
	})
}

func (s *Server) handleWebSocket(c *gin.Context) {
	websocket.ServeWS(c.Writer, c.Request, s.hub)
}

func (s *Server) searchManga(ctx context.Context, query, genre string) ([]models.Manga, error) {
	grpcGenre := normalizeGenreFilter(genre)
	resp, err := s.grpcClient.SearchManga(ctx, &proto.SearchRequest{Query: query, Genre: grpcGenre})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return []models.Manga{}, nil
		}
		return nil, err
	}

	results := make([]models.Manga, 0, len(resp.Results))
	for _, item := range resp.Results {
		results = append(results, mangaFromProto(item))
	}
	return results, nil
}

func normalizeGenreFilter(genre string) string {
	trimmed := strings.TrimSpace(genre)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "[") {
		return trimmed
	}

	parts := strings.Split(trimmed, ",")
	genres := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			genres = append(genres, value)
		}
	}
	if len(genres) == 0 {
		return ""
	}

	payload, err := json.Marshal(genres)
	if err != nil {
		return ""
	}
	return string(payload)
}

func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func mangaFromProto(resp *proto.MangaResponse) models.Manga {
	if resp == nil {
		return models.Manga{}
	}
	return models.Manga{
		ID:            resp.Id,
		Title:         resp.Title,
		Author:        resp.Author,
		Genres:        resp.Genres,
		Status:        resp.Status,
		TotalChapters: int(resp.TotalChapters),
		Description:   resp.Description,
		CoverURL:      resp.CoverUrl,
	}
}

func (s *Server) generateToken(userID, username string) (string, error) {
	claims := &jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(24 * time.Hour)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userID":   userID,
		"username": username,
		"exp":      claims.ExpiresAt.Unix(),
		"iat":      claims.IssuedAt.Unix(),
		"sub":      claims.Subject,
	})

	return token.SignedString(s.jwtSecret)
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		tokenStr := ""
		if header != "" {
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
				return
			}
			tokenStr = parts[1]
		} else if c.Request.URL.Path == "/ws" {
			tokenStr = c.Query("token")
			if tokenStr == "" {
				tokenStr = c.Query("access_token")
			}
			if tokenStr == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization token required"})
				return
			}
		} else {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			return
		}

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return s.jwtSecret, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}

		userID, _ := claims["userID"].(string)
		username, _ := claims["username"].(string)
		if userID == "" || username == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token missing user identity"})
			return
		}

		c.Set("userID", userID)
		c.Set("username", username)

		ctx := context.WithValue(c.Request.Context(), "userID", userID)
		ctx = context.WithValue(ctx, "username", username)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
