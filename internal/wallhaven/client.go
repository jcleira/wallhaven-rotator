// Package wallhaven is a thin HTTP client for the Wallhaven public API.
//
// See https://wallhaven.cc/help/api. The SFW API does not require
// authentication; sketchy/NSFW does. v1 targets SFW only.
package wallhaven

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiBase      = "https://wallhaven.cc/api/v1"
	userAgent    = "wallhaven-rotator/0.1 (+https://github.com/jcleira/wallhaven-rotator)"
	httpTimeout  = 30 * time.Second
)

type Client struct {
	HTTP    *http.Client
	APIKey  string // optional; unused for SFW
	BaseURL string // override for testing
}

func New() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: httpTimeout},
		BaseURL: apiBase,
	}
}

// SearchParams mirrors the Wallhaven /search query parameters we care about.
// All fields are optional; zero values mean "don't send".
type SearchParams struct {
	Query      string   // freeform search, maps to ?q=
	Categories string   // 3-char bitmap, e.g. "100" = general only
	Purity     string   // 3-char bitmap, e.g. "100" = SFW only
	Sorting    string   // hot | toplist | views | favorites | random | date_added | relevance
	Order      string   // asc | desc
	AtLeast    string   // e.g. "3840x1600"
	Ratios     []string // e.g. ["21x9", "32x9"]
	Colors     []string // hex codes without '#'
	Page       int      // 1-based
	Seed       string   // stability for random sort
}

// Wallpaper is one entry from the Wallhaven search response.
type Wallpaper struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	ShortURL   string   `json:"short_url"`
	Views      int      `json:"views"`
	Favorites  int      `json:"favorites"`
	Source     string   `json:"source"`
	Purity     string   `json:"purity"`
	Category   string   `json:"category"`
	DimensionX int      `json:"dimension_x"`
	DimensionY int      `json:"dimension_y"`
	Resolution string   `json:"resolution"`
	Ratio      string   `json:"ratio"`
	FileSize   int64    `json:"file_size"`
	FileType   string   `json:"file_type"`
	CreatedAt  string   `json:"created_at"`
	Colors     []string `json:"colors"`
	Path       string   `json:"path"` // direct image URL
	Thumbs     struct {
		Large    string `json:"large"`
		Original string `json:"original"`
		Small    string `json:"small"`
	} `json:"thumbs"`
}

type SearchMeta struct {
	CurrentPage int    `json:"current_page"`
	LastPage    int    `json:"last_page"`
	PerPage     int    `json:"per_page"`
	Total       int    `json:"total"`
	Seed        string `json:"seed"`
}

type SearchResponse struct {
	Data []Wallpaper `json:"data"`
	Meta SearchMeta  `json:"meta"`
}

// Search queries /api/v1/search with the given params.
func (c *Client) Search(ctx context.Context, p SearchParams) (*SearchResponse, error) {
	q := url.Values{}
	if p.Query != "" {
		q.Set("q", p.Query)
	}
	if p.Categories != "" {
		q.Set("categories", p.Categories)
	}
	if p.Purity != "" {
		q.Set("purity", p.Purity)
	}
	if p.Sorting != "" {
		q.Set("sorting", p.Sorting)
	}
	if p.Order != "" {
		q.Set("order", p.Order)
	}
	if p.AtLeast != "" {
		q.Set("atleast", p.AtLeast)
	}
	if len(p.Ratios) > 0 {
		q.Set("ratios", strings.Join(p.Ratios, ","))
	}
	if len(p.Colors) > 0 {
		q.Set("colors", strings.Join(p.Colors, ","))
	}
	if p.Page > 0 {
		q.Set("page", fmt.Sprintf("%d", p.Page))
	}
	if p.Seed != "" {
		q.Set("seed", p.Seed)
	}
	if c.APIKey != "" {
		q.Set("apikey", c.APIKey)
	}

	endpoint := c.BaseURL + "/search?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wallhaven GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("wallhaven rate limited (429); back off and retry later")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("wallhaven %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &out, nil
}

// Download streams the image at path into w. Sets the UA header.
func (c *Client) Download(ctx context.Context, path string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("wallhaven download %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wallhaven download %d on %s", resp.StatusCode, path)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("stream image: %w", err)
	}
	return nil
}
