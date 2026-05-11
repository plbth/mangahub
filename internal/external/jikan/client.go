package jikan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/plbth/mangahub/pkg/models"
)

const defaultBaseURL = "https://api.jikan.moe/v4"

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) SearchManga(ctx context.Context, title string, limit int) ([]models.Manga, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("jikan: title is required")
	}
	if limit <= 0 || limit > 25 {
		limit = 5
	}

	values := url.Values{}
	values.Set("q", title)
	values.Set("limit", strconv.Itoa(limit))
	values.Set("sfw", "true")
	values.Set("order_by", "members")
	values.Set("sort", "desc")

	endpoint := strings.TrimRight(c.baseURL, "/") + "/manga?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("jikan: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MangaHub coursework demo (github.com/plbth/mangahub)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jikan: search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jikan: search returned %s", resp.Status)
	}

	var payload jikanSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("jikan: decode response: %w", err)
	}

	results := make([]models.Manga, 0, len(payload.Data))
	for _, item := range payload.Data {
		results = append(results, item.toModel())
	}
	return results, nil
}

type jikanSearchResponse struct {
	Data []jikanManga `json:"data"`
}

type jikanManga struct {
	MALID    int          `json:"mal_id"`
	Title    string       `json:"title"`
	TitleEng string       `json:"title_english"`
	Images   jikanImages  `json:"images"`
	Chapters int          `json:"chapters"`
	Status   string       `json:"status"`
	Synopsis string       `json:"synopsis"`
	Authors  []jikanNamed `json:"authors"`
	Genres   []jikanNamed `json:"genres"`
}

type jikanImages struct {
	JPG jikanImageURLs `json:"jpg"`
}

type jikanImageURLs struct {
	ImageURL      string `json:"image_url"`
	LargeImageURL string `json:"large_image_url"`
}

type jikanNamed struct {
	Name string `json:"name"`
}

func (m jikanManga) toModel() models.Manga {
	return models.Manga{
		ID:            fmt.Sprintf("jikan-%d", m.MALID),
		Title:         firstNonEmpty(m.TitleEng, m.Title),
		Author:        namedList(m.Authors),
		Genres:        namedSlice(m.Genres),
		Status:        normalizeStatus(m.Status),
		TotalChapters: m.Chapters,
		Description:   m.Synopsis,
		CoverURL:      firstNonEmpty(m.Images.JPG.LargeImageURL, m.Images.JPG.ImageURL),
	}
}

func namedList(items []jikanNamed) string {
	names := namedSlice(items)
	if len(names) == 0 {
		return "Unknown"
	}
	return strings.Join(names, ", ")
}

func namedSlice(items []jikanNamed) []string {
	values := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		key := strings.ToLower(name)
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		values = append(values, name)
	}
	return values
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "finished", "complete", "completed":
		return "completed"
	case "publishing", "currently publishing":
		return "ongoing"
	default:
		if strings.TrimSpace(status) == "" {
			return "unknown"
		}
		return strings.ToLower(status)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
