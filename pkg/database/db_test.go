package database

import (
    "testing"
    "github.com/plbth/mangahub/pkg/models"
)

func TestDatabaseOperations(t *testing.T) {
    // Setup: create temp database
    dbPath := ":memory:" // SQLite in-memory database
    db, err := InitDB(dbPath)
    if err != nil {
        t.Fatalf("Failed to init DB: %v", err)
    }
    defer db.Close()

    repo := NewSQLiteRepository(db)

    // Shared state for tests
    var createdUserID string
    var createdMangaID = "one-piece"

    // Test 1: Create User
    t.Run("CreateUser", func(t *testing.T) {
        user := &models.User{
            Username:     "testuser",
            Email:        "test@example.com",
            PasswordHash: "plaintextpassword123", // Will be hashed
        }
        err := repo.CreateUser(user)
        if err != nil {
            t.Fatalf("CreateUser failed: %v", err)
        }
        if user.ID == "" {
            t.Error("User ID should not be empty after creation")
        }
        createdUserID = user.ID // CAPTURE the actual UUID
        t.Logf("✓ User created: %s (ID: %s)", user.Username, user.ID)
    })

    // Test 2: Get User
    t.Run("GetUserByUsername", func(t *testing.T) {
        user, err := repo.GetUserByUsername("testuser")
        if err != nil {
            t.Fatalf("GetUserByUsername failed: %v", err)
        }
        if user.Username != "testuser" {
            t.Errorf("Expected username 'testuser', got '%s'", user.Username)
        }
        t.Logf("✓ User retrieved: %s", user.Username)
    })

    // Test 3: Add Manga
    t.Run("AddManga", func(t *testing.T) {
        manga := &models.Manga{
            ID:            createdMangaID,
            Title:         "One Piece",
            Author:        "Eiichiro Oda",
            Genres:        []string{"action", "adventure"},
            Status:        "ongoing",
            TotalChapters: 1100,
            Description:   "A pirate adventure",
            CoverURL:      "https://example.com/one-piece.jpg",
        }
        err := repo.AddManga(manga)
        if err != nil {
            t.Fatalf("AddManga failed: %v", err)
        }
        t.Logf("✓ Manga added: %s", manga.Title)
    })

    // Test 4: Get Manga
    t.Run("GetManga", func(t *testing.T) {
        manga, err := repo.GetManga(createdMangaID)
        if err != nil {
            t.Fatalf("GetManga failed: %v", err)
        }
        if manga.Title != "One Piece" {
            t.Errorf("Expected 'One Piece', got '%s'", manga.Title)
        }
        if len(manga.Genres) != 2 {
            t.Errorf("Expected 2 genres, got %d", len(manga.Genres))
        }
        t.Logf("✓ Manga retrieved: %s (Genres: %v)", manga.Title, manga.Genres)
    })

    // Test 5: Search Manga
    t.Run("SearchManga", func(t *testing.T) {
        results, err := repo.SearchManga("piece", "")
        if err != nil {
            t.Fatalf("SearchManga failed: %v", err)
        }
        if len(results) == 0 {
            t.Error("SearchManga should return at least one result")
        }
        t.Logf("✓ Search found %d results", len(results))
    })

    // Test 6: Update Progress
    t.Run("UpdateProgress", func(t *testing.T) {
        // Use the actual created user ID
        progress := &models.UserProgress{
            UserID:         createdUserID,
            MangaID:        createdMangaID,
            CurrentChapter: 100,
            Volume:         10,
            Status:         "reading",
            Rating:         8,
            Notes:          "Great series!",
        }
        err := repo.UpdateProgress(progress)
        if err != nil {
            t.Fatalf("UpdateProgress failed: %v", err)
        }
        t.Logf("✓ Progress updated: Chapter %d", progress.CurrentChapter)
    })

    // Test 7: Get User Library
    t.Run("GetUserLibrary", func(t *testing.T) {
        // Use the actual created user ID
        library, err := repo.GetUserLibrary(createdUserID)
        if err != nil {
            t.Fatalf("GetUserLibrary failed: %v", err)
        }
        if len(library) == 0 {
            t.Error("User library should have at least one entry")
        }
        t.Logf("✓ Library retrieved: %d entries", len(library))
    })
}