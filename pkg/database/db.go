package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	sqlite3 "github.com/mattn/go-sqlite3"
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

// SeedMangaFromJSON loads manga data when the manga table is empty.
func SeedMangaFromJSON(db *sql.DB, repo Repository, path string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("seed manga: db is nil")
	}
	if repo == nil {
		return 0, fmt.Errorf("seed manga: repo is nil")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM manga;`).Scan(&count); err != nil {
		return 0, fmt.Errorf("seed manga: count failed: %w", err)
	}
	if count > 0 {
		return 0, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("seed manga: read %s failed: %w", path, err)
	}

	var items []models.Manga
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, fmt.Errorf("seed manga: parse %s failed: %w", path, err)
	}

	inserted := 0
	for _, item := range items {
		manga := item
		if err := repo.AddManga(&manga); err != nil {
			return inserted, fmt.Errorf("seed manga: insert %s failed: %w", manga.ID, err)
		}
		inserted++
	}

	return inserted, nil
}

// Example dummy implementation to satisfy the interface for now
func (s *SQLiteDB) CreateUser(user *models.User) error {
	if user == nil {
		return fmt.Errorf("failed to create user: user is nil")
	}

	user.ID = uuid.NewString()
	user.CreatedAt = time.Now().UTC()

	_, err := s.db.Exec(
		`INSERT INTO users (id, username, email, password_hash, created_at)
		 VALUES (?, ?, ?, ?, ?);`,
		user.ID,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.CreatedAt,
	)
	if err != nil {
		var sqlErr sqlite3.Error
		if errors.As(err, &sqlErr) && sqlErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return fmt.Errorf("failed to create user: username or email already exists")
		}
		log.Printf("[DATABASE] failed to create user: %v", err)
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (s *SQLiteDB) GetUserByUsername(username string) (*models.User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, email, password_hash, created_at
		 FROM users
		 WHERE username = ?;`,
		username,
	)

	var user models.User
	if err := row.Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		log.Printf("[DATABASE] failed to get user by username: %v", err)
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return &user, nil
}

func (s *SQLiteDB) AddManga(manga *models.Manga) error {
	if manga == nil {
		return fmt.Errorf("failed to add manga: manga is nil")
	}

	genresJSON, err := json.Marshal(manga.Genres)
	if err != nil {
		log.Printf("[DATABASE] failed to marshal genres: %v", err)
		return fmt.Errorf("failed to add manga: could not serialize genres")
	}

	_, err = s.db.Exec(
		`INSERT INTO manga (id, title, author, genres, status, total_chapters, description, cover_url)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?);`,
		manga.ID,
		manga.Title,
		manga.Author,
		string(genresJSON),
		manga.Status,
		manga.TotalChapters,
		manga.Description,
		manga.CoverURL,
	)
	if err != nil {
		log.Printf("[DATABASE] failed to add manga: %v", err)
		return fmt.Errorf("failed to add manga: %w", err)
	}

	return nil
}

func (s *SQLiteDB) GetManga(id string) (*models.Manga, error) {
	row := s.db.QueryRow(
		`SELECT id, title, author, genres, status, total_chapters, description, cover_url
		 FROM manga
		 WHERE id = ?;`,
		id,
	)

	var manga models.Manga
	var genresJSON string
	if err := row.Scan(
		&manga.ID,
		&manga.Title,
		&manga.Author,
		&genresJSON,
		&manga.Status,
		&manga.TotalChapters,
		&manga.Description,
		&manga.CoverURL,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		log.Printf("[DATABASE] failed to get manga: %v", err)
		return nil, fmt.Errorf("failed to get manga: %w", err)
	}

	if genresJSON != "" {
		if err := json.Unmarshal([]byte(genresJSON), &manga.Genres); err != nil {
			log.Printf("[DATABASE] failed to parse genres: %v", err)
			return nil, fmt.Errorf("failed to get manga: could not parse genres")
		}
	}

	return &manga, nil
}

func (s *SQLiteDB) SearchManga(query string, genre string) ([]models.Manga, error) {
	search := strings.TrimSpace(query)
	if search == "" {
		search = "%"
	} else {
		search = "%" + search + "%"
	}

	args := []interface{}{search, search}
	conditions := []string{"(LOWER(title) LIKE LOWER(?) OR LOWER(author) LIKE LOWER(?))"}

	if strings.TrimSpace(genre) != "" {
		var genres []string
		if err := json.Unmarshal([]byte(genre), &genres); err != nil {
			log.Printf("[DATABASE] failed to parse genre filter: %v", err)
			return nil, fmt.Errorf("failed to search manga: invalid genre filter")
		}
		for _, g := range genres {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			conditions = append(conditions, "LOWER(genres) LIKE LOWER(?)")
			args = append(args, "%"+g+"%")
		}
	}

	querySQL := fmt.Sprintf(
		`SELECT id, title, author, genres, status, total_chapters, description, cover_url
		 FROM manga
		 WHERE %s
		 LIMIT 20;`,
		strings.Join(conditions, " AND "),
	)

	rows, err := s.db.Query(querySQL, args...)
	if err != nil {
		log.Printf("[DATABASE] failed to search manga: %v", err)
		return nil, fmt.Errorf("failed to search manga: %w", err)
	}
	defer rows.Close()

	var results []models.Manga
	for rows.Next() {
		var manga models.Manga
		var genresJSON string
		if err := rows.Scan(
			&manga.ID,
			&manga.Title,
			&manga.Author,
			&genresJSON,
			&manga.Status,
			&manga.TotalChapters,
			&manga.Description,
			&manga.CoverURL,
		); err != nil {
			log.Printf("[DATABASE] failed to scan manga row: %v", err)
			return nil, fmt.Errorf("failed to search manga: %w", err)
		}

		if genresJSON != "" {
			if err := json.Unmarshal([]byte(genresJSON), &manga.Genres); err != nil {
				log.Printf("[DATABASE] failed to parse genres: %v", err)
				return nil, fmt.Errorf("failed to search manga: could not parse genres")
			}
		}

		results = append(results, manga)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[DATABASE] row iteration error: %v", err)
		return nil, fmt.Errorf("failed to search manga: %w", err)
	}

	return results, nil
}

func (s *SQLiteDB) UpdateProgress(progress *models.UserProgress) error {
	if progress == nil {
		return fmt.Errorf("failed to update progress: progress is nil")
	}

	_, err := s.db.Exec(
		`INSERT INTO user_progress (user_id, manga_id, current_chapter, volume, status, rating, notes, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id, manga_id) DO UPDATE SET
			current_chapter = excluded.current_chapter,
			volume = excluded.volume,
			status = excluded.status,
			rating = excluded.rating,
			notes = excluded.notes,
			updated_at = CURRENT_TIMESTAMP;`,
		progress.UserID,
		progress.MangaID,
		progress.CurrentChapter,
		progress.Volume,
		progress.Status,
		progress.Rating,
		progress.Notes,
	)
	if err != nil {
		log.Printf("[DATABASE] failed to update progress: %v", err)
		return fmt.Errorf("failed to update progress: %w", err)
	}

	return nil
}

func (s *SQLiteDB) GetUserLibrary(userID string) ([]models.LibraryView, error) {
	rows, err := s.db.Query(
		`SELECT
			m.id,
			m.title,
			m.author,
			m.genres,
			m.status,
			m.total_chapters,
			m.description,
			m.cover_url,
			up.user_id,
			up.manga_id,
			COALESCE(up.current_chapter, 0),
			COALESCE(up.volume, 0),
			up.status,
			COALESCE(up.rating, 0),
			up.notes,
			up.updated_at
		 FROM user_progress up
		 INNER JOIN users u ON u.id = up.user_id
		 INNER JOIN manga m ON m.id = up.manga_id
		 WHERE up.user_id = ?
		 ORDER BY up.updated_at DESC;`,
		userID,
	)
	if err != nil {
		log.Printf("[DATABASE] failed to get user library: %v", err)
		return nil, fmt.Errorf("failed to get user library: %w", err)
	}
	defer rows.Close()

	var library []models.LibraryView
	for rows.Next() {
		var view models.LibraryView
		var genresJSON string
		var notes sql.NullString
		var status sql.NullString
		var updatedAt sql.NullTime

		if err := rows.Scan(
			&view.Manga.ID,
			&view.Manga.Title,
			&view.Manga.Author,
			&genresJSON,
			&view.Manga.Status,
			&view.Manga.TotalChapters,
			&view.Manga.Description,
			&view.Manga.CoverURL,
			&view.Progress.UserID,
			&view.Progress.MangaID,
			&view.Progress.CurrentChapter,
			&view.Progress.Volume,
			&status,
			&view.Progress.Rating,
			&notes,
			&updatedAt,
		); err != nil {
			log.Printf("[DATABASE] failed to scan user library row: %v", err)
			return nil, fmt.Errorf("failed to get user library: %w", err)
		}

		if genresJSON != "" {
			if err := json.Unmarshal([]byte(genresJSON), &view.Manga.Genres); err != nil {
				log.Printf("[DATABASE] failed to parse genres: %v", err)
				return nil, fmt.Errorf("failed to get user library: could not parse genres")
			}
		}

		if status.Valid {
			view.Progress.Status = status.String
		}
		if notes.Valid {
			view.Progress.Notes = notes.String
		}
		if updatedAt.Valid {
			view.Progress.UpdatedAt = updatedAt.Time
		}

		library = append(library, view)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[DATABASE] row iteration error: %v", err)
		return nil, fmt.Errorf("failed to get user library: %w", err)
	}

	return library, nil
}