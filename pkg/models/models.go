package models

import "time"

// ==========================================
// 1. Core Database Entities
// ==========================================

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Never expose in JSON
	CreatedAt    time.Time `json:"created_at"`
}

type Manga struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Genres        []string `json:"genres"` // Stored as JSON string in DB
	Status        string   `json:"status"` // ongoing, completed
	TotalChapters int      `json:"total_chapters"`
	Description   string   `json:"description"`
	CoverURL      string   `json:"cover_url,omitempty"`
}

type UserProgress struct {
	UserID         string    `json:"user_id"`
	MangaID        string    `json:"manga_id"`
	CurrentChapter int       `json:"current_chapter"`
	Volume         int       `json:"volume,omitempty"` // From CLI spec
	Status         string    `json:"status"`           // reading, completed, plan-to-read, on-hold, dropped
	Rating         int       `json:"rating,omitempty"` // 1-10
	Notes          string    `json:"notes,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// LibraryView represents a manga combined with the user's progress
type LibraryView struct {
	Manga    Manga        `json:"manga"`
	Progress UserProgress `json:"progress"`
}

// ==========================================
// 2. HTTP API Payloads (Person B)
// ==========================================

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"` // CLI allows login with email too
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// ==========================================
// 3. Network Protocol Payloads
// ==========================================

// TCP Sync Protocol
type ProgressUpdate struct {
	UserID    string `json:"user_id"`
	MangaID   string `json:"manga_id"`
	Chapter   int    `json:"chapter"`
	Timestamp int64  `json:"timestamp"`
}

// UDP Notification Protocol
type Notification struct {
	Type      string `json:"type"` // e.g., "new_chapter", "system"
	MangaID   string `json:"manga_id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// WebSocket Chat Protocol
type ChatMessage struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	MangaID   string `json:"manga_id,omitempty"` // Empty if general chat
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}