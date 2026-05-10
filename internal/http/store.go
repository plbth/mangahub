package httpapi

import (
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/plbth/mangahub/pkg/models"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicateUser = errors.New("username or email already exists")
)

// MemoryStore keeps the HTTP layer fully testable without the database layer.
type MemoryStore struct {
	mu            sync.RWMutex
	usersByID     map[string]*models.User
	usersByName   map[string]*models.User
	usersByEmail  map[string]*models.User
	mangasByID    map[string]models.Manga
	progressByUID map[string]map[string]models.UserProgress // userID -> mangaID -> progress
}

func NewMemoryStore(mangas []models.Manga) *MemoryStore {
	m := &MemoryStore{
		usersByID:     make(map[string]*models.User),
		usersByName:   make(map[string]*models.User),
		usersByEmail:  make(map[string]*models.User),
		mangasByID:    make(map[string]models.Manga, len(mangas)),
		progressByUID: make(map[string]map[string]models.UserProgress),
	}
	for _, manga := range mangas {
		m.mangasByID[manga.ID] = manga
	}
	return m
}

func (m *MemoryStore) CreateUser(user *models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.usersByName[strings.ToLower(user.Username)]; ok {
		return ErrDuplicateUser
	}
	if _, ok := m.usersByEmail[strings.ToLower(user.Email)]; ok {
		return ErrDuplicateUser
	}

	clone := *user
	m.usersByID[user.ID] = &clone
	m.usersByName[strings.ToLower(user.Username)] = &clone
	m.usersByEmail[strings.ToLower(user.Email)] = &clone
	return nil
}

func (m *MemoryStore) GetUserByUsername(username string) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.usersByName[strings.ToLower(username)]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *user
	return &clone, nil
}

func (m *MemoryStore) GetUserByEmail(email string) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.usersByEmail[strings.ToLower(email)]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *user
	return &clone, nil
}

func (m *MemoryStore) GetManga(id string) (*models.Manga, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	manga, ok := m.mangasByID[id]
	if !ok {
		return nil, ErrNotFound
	}
	clone := manga
	return &clone, nil
}

func (m *MemoryStore) SearchManga(query string, genre string, status string) []models.Manga {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query = strings.ToLower(strings.TrimSpace(query))
	genre = strings.ToLower(strings.TrimSpace(genre))
	status = strings.ToLower(strings.TrimSpace(status))

	results := make([]models.Manga, 0)
	for _, manga := range m.mangasByID {
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{manga.Title, manga.Author, manga.Description}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}

		if genre != "" && !containsGenre(manga.Genres, genre) {
			continue
		}

		if status != "" && !strings.EqualFold(manga.Status, status) {
			continue
		}

		results = append(results, manga)
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Title) < strings.ToLower(results[j].Title)
	})
	return results
}

func (m *MemoryStore) UpsertProgress(progress models.UserProgress) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.progressByUID[progress.UserID]; !ok {
		m.progressByUID[progress.UserID] = make(map[string]models.UserProgress)
	}
	m.progressByUID[progress.UserID][progress.MangaID] = progress
	return nil
}

func (m *MemoryStore) GetProgress(userID, mangaID string) (models.UserProgress, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	byUser, ok := m.progressByUID[userID]
	if !ok {
		return models.UserProgress{}, false
	}
	progress, ok := byUser[mangaID]
	return progress, ok
}

func (m *MemoryStore) GetUserLibrary(userID string) []models.LibraryView {
	m.mu.RLock()
	defer m.mu.RUnlock()

	byUser, ok := m.progressByUID[userID]
	if !ok {
		return []models.LibraryView{}
	}

	views := make([]models.LibraryView, 0, len(byUser))
	for mangaID, progress := range byUser {
		manga, ok := m.mangasByID[mangaID]
		if !ok {
			continue
		}
		views = append(views, models.LibraryView{Manga: manga, Progress: progress})
	}

	sort.Slice(views, func(i, j int) bool {
		return strings.ToLower(views[i].Manga.Title) < strings.ToLower(views[j].Manga.Title)
	})
	return views
}

func containsGenre(genres []string, want string) bool {
	for _, g := range genres {
		if strings.EqualFold(g, want) {
			return true
		}
	}
	return false
}
