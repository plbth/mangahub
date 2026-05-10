package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type APIError struct {
	Error string `json:"error"`
}

type HTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newHTTPClient() *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(cfg.APIBaseURL, "/"),
		token:   strings.TrimSpace(cfg.Token),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (h *HTTPClient) setToken(token string) {
	h.token = strings.TrimSpace(token)
}

func (h *HTTPClient) request(method, endpoint string, body any, out any, auth bool) error {
	fullURL := strings.TrimRight(h.baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, fullURL, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		if strings.TrimSpace(h.token) == "" {
			return fmt.Errorf("missing token; run 'mangahub auth login' first or use --token")
		}
		req.Header.Set("Authorization", "Bearer "+h.token)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		var apiErr APIError
		if json.Unmarshal(raw, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("request failed: %s", strings.TrimSpace(string(raw)))
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (h *HTTPClient) ping() error {
	fullURL := strings.TrimRight(h.baseURL, "/") + "/health"
	resp, err := h.client.Get(fullURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("health check returned %s", resp.Status)
	}
	return nil
}
