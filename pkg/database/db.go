package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
	"github.com/plbth/mangahub/pkg/models"
)

// DB Repository Interface
type Repository interface {
	// Auth
	CreateUser(user *models.User) error
	GetUserByUsername(username string) (*models.User, error)
	
	// Manga
	AddManga(manga *models.Manga) error
	GetManga(id string) (*models.Manga, error)
	SearchManga(query string, genre string) ([]models.Manga, error)
	
	// Library
	UpdateProgress(progress *models.UserProgress) error
	GetUserLibrary(userID string) ([]models.LibraryView, error)
}

// DB Wrapper
type SQLiteDB struct {
	db *sql.DB
}

// InitDB connects to SQLite and ensures tables exist
func InitDB(filepath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign key support in SQLite
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	log.Println("Database initialized successfully at", filepath)
	return db, nil
}

func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            TEXT PRIMARY KEY,
			username      TEXT UNIQUE NOT NULL,
			email         TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS manga (
			id             TEXT PRIMARY KEY,
			title          TEXT NOT NULL,
			author         TEXT,
			genres         TEXT, -- Stored as JSON array string
			status         TEXT,
			total_chapters INTEGER,
			description    TEXT,
			cover_url      TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS user_progress (
			user_id         TEXT,
			manga_id        TEXT,
			current_chapter INTEGER DEFAULT 0,
			volume          INTEGER DEFAULT 0,
			status          TEXT,
			rating          INTEGER DEFAULT 0,
			notes           TEXT,
			updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, manga_id),
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY(manga_id) REFERENCES manga(id) ON DELETE CASCADE
		);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

// Implement the Repository interface
func NewSQLiteRepository(db *sql.DB) Repository {
	return &SQLiteDB{db: db}
}

// Example dummy implementation to satisfy the interface for now
func (s *SQLiteDB) CreateUser(user *models.User) error { return nil }
func (s *SQLiteDB) GetUserByUsername(username string) (*models.User, error) { return nil, nil }
func (s *SQLiteDB) AddManga(manga *models.Manga) error { return nil }
func (s *SQLiteDB) GetManga(id string) (*models.Manga, error) { return nil, nil }
func (s *SQLiteDB) SearchManga(query string, genre string) ([]models.Manga, error) { return nil, nil }
func (s *SQLiteDB) UpdateProgress(progress *models.UserProgress) error { return nil }
func (s *SQLiteDB) GetUserLibrary(userID string) ([]models.LibraryView, error) { return nil, nil }