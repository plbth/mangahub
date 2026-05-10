package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/plbth/mangahub/pkg/models"
)

// Config groups the inputs needed by the HTTP API.
type Config struct {
	DataPath  string
	JWTSecret string
}

// Server bundles the router and in-memory storage so the API can run
// independently from the unfinished database layer.
type Server struct {
	router    *gin.Engine
	store     *MemoryStore
	jwtSecret []byte
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.DataPath == "" {
		cfg.DataPath = "data/manga.json"
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "mangahub-dev-secret"
	}

	mangas, err := LoadMangaData(cfg.DataPath)
	if err != nil {
		return nil, err
	}

	store := NewMemoryStore(mangas)
	s := &Server{
		store:     store,
		jwtSecret: []byte(cfg.JWTSecret),
	}
	s.router = s.buildRouter()
	return s, nil
}

func (s *Server) Router() *gin.Engine {
	return s.router
}

func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

func (s *Server) buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), corsMiddleware())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"service":   "mangahub-http",
			"timestamp": time.Now().UTC(),
		})
	})

	auth := r.Group("/auth")
	{
		auth.POST("/register", s.handleRegister)
		auth.POST("/login", s.handleLogin)
	}

	manga := r.Group("/manga")
	{
		manga.GET("", s.handleListManga)
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

func (s *Server) handleRegister(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := &models.User{
		ID:           newID("usr"),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.store.CreateUser(user); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrDuplicateUser) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	token, err := s.generateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusCreated, models.AuthResponse{
		Token: token,
		User:  *sanitizeUser(user),
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user *models.User
	var err error
	if req.Username != "" {
		user, err = s.store.GetUserByUsername(req.Username)
	} else if req.Email != "" {
		user, err = s.store.GetUserByEmail(req.Email)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username or email is required"})
		return
	}
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := s.generateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, models.AuthResponse{
		Token: token,
		User:  *sanitizeUser(user),
	})
}

func (s *Server) handleListManga(c *gin.Context) {
	query := strings.TrimSpace(c.Query("query"))
	genre := strings.TrimSpace(c.Query("genre"))
	status := strings.TrimSpace(c.Query("status"))

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

	results := s.store.SearchManga(query, genre, status)
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

	c.JSON(http.StatusOK, gin.H{
		"count":  len(results),
		"manga":  results,
		"query":  query,
		"genre":  genre,
		"status": status,
	})
}

func (s *Server) handleGetManga(c *gin.Context) {
	manga, err := s.store.GetManga(c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, manga)
}

func (s *Server) handleAddToLibrary(c *gin.Context) {
	userID, _ := c.Get("userID")
	username, _ := c.Get("username")

	var req AddLibraryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manga, err := s.store.GetManga(req.MangaID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	progress := models.UserProgress{
		UserID:         userID.(string),
		MangaID:        manga.ID,
		CurrentChapter: req.CurrentChapter,
		Volume:         req.Volume,
		Status:         req.Status,
		Rating:         req.Rating,
		Notes:          req.Notes,
		UpdatedAt:      time.Now().UTC(),
	}

	if err := s.store.UpsertProgress(progress); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "added to library",
		"user":     username,
		"manga":    manga.Title,
		"progress": progress,
	})
}

func (s *Server) handleGetLibrary(c *gin.Context) {
	userID, _ := c.Get("userID")
	status := strings.TrimSpace(c.Query("status"))

	library := s.store.GetUserLibrary(userID.(string))
	if status != "" {
		filtered := make([]models.LibraryView, 0, len(library))
		for _, item := range library {
			if strings.EqualFold(item.Progress.Status, status) {
				filtered = append(filtered, item)
			}
		}
		library = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"count":   len(library),
		"library": library,
	})
}

func (s *Server) handleUpdateProgress(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req UpdateProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manga, err := s.store.GetManga(req.MangaID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if manga.TotalChapters > 0 && req.Chapter > manga.TotalChapters {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("chapter %d exceeds manga's total chapters (%d)", req.Chapter, manga.TotalChapters),
		})
		return
	}

	existing, ok := s.store.GetProgress(userID.(string), manga.ID)
	if ok && req.Chapter < existing.CurrentChapter {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("chapter %d is behind your current progress (chapter %d)", req.Chapter, existing.CurrentChapter),
		})
		return
	}

	progress := models.UserProgress{
		UserID:         userID.(string),
		MangaID:        manga.ID,
		CurrentChapter: req.Chapter,
		Volume:         req.Volume,
		Status:         "reading",
		Rating:         req.Rating,
		Notes:          req.Notes,
		UpdatedAt:      time.Now().UTC(),
	}

	if err := s.store.UpsertProgress(progress); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "progress updated successfully",
		"manga":    manga.Title,
		"progress": progress,
	})
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
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			return
		}

		token, err := jwt.Parse(parts[1], func(token *jwt.Token) (any, error) {
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

// LoadMangaData reads the JSON seed file from data/manga.json.
func LoadMangaData(path string) ([]models.Manga, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manga data: %w", err)
	}

	var mangas []models.Manga
	if err := json.Unmarshal(raw, &mangas); err != nil {
		return nil, fmt.Errorf("parse manga data: %w", err)
	}
	return mangas, nil
}
