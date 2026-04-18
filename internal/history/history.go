// Package history persists the set of wallpapers we've already shown so we
// never re-pick one.
package history

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Entry struct {
	ID          string
	URL         string
	ImageURL    string
	Ratio       string
	Width       int
	Height      int
	Tags        string
	ShownOn     string // YYYY-MM-DD
	FavoritedAt string // empty if not favorited
}

type Store struct {
	db *sql.DB
}

// Open opens (or creates) the history DB at configDir/history.db.
func Open(configDir string) (*Store, error) {
	path := filepath.Join(configDir, "history.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Record saves a wallpaper as shown today. Safe to call multiple times per day.
func (s *Store) Record(e Entry) error {
	if e.ShownOn == "" {
		e.ShownOn = time.Now().Format("2006-01-02")
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO shown (id, shown_on, url, image_url, ratio, width, height, tags, favorited_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?,
		   COALESCE((SELECT favorited_at FROM shown WHERE id = ?), NULL))`,
		e.ID, e.ShownOn, e.URL, e.ImageURL, e.Ratio, e.Width, e.Height, e.Tags, e.ID,
	)
	return err
}

// Seen returns true if the given wallhaven id has been shown before.
func (s *Store) Seen(id string) (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM shown WHERE id = ?`, id).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// MarkFavorited marks the given id as favorited now.
func (s *Store) MarkFavorited(id string) error {
	_, err := s.db.Exec(
		`UPDATE shown SET favorited_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), id,
	)
	return err
}

func (s *Store) MarkUnfavorited(id string) error {
	_, err := s.db.Exec(`UPDATE shown SET favorited_at = NULL WHERE id = ?`, id)
	return err
}

// Get returns a single entry or sql.ErrNoRows.
func (s *Store) Get(id string) (Entry, error) {
	var e Entry
	var fav sql.NullString
	err := s.db.QueryRow(
		`SELECT id, shown_on, url, image_url, ratio, width, height, tags, favorited_at
		 FROM shown WHERE id = ?`, id,
	).Scan(&e.ID, &e.ShownOn, &e.URL, &e.ImageURL, &e.Ratio, &e.Width, &e.Height, &e.Tags, &fav)
	if err != nil {
		return Entry{}, err
	}
	if fav.Valid {
		e.FavoritedAt = fav.String
	}
	return e, nil
}

// Count returns total history rows.
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM shown`).Scan(&n)
	return n, err
}

// FavoriteCount returns count of rows with favorited_at set.
func (s *Store) FavoriteCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM shown WHERE favorited_at IS NOT NULL`).Scan(&n)
	return n, err
}

// SeenIDs returns the full set of seen ids. Caller typically uses this as an
// in-memory skip list when paging search results.
func (s *Store) SeenIDs() (map[string]struct{}, error) {
	rows, err := s.db.Query(`SELECT id FROM shown`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// TagsToString joins a tag list for storage. Trims whitespace.
func TagsToString(tags []string) string {
	cleaned := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			cleaned = append(cleaned, t)
		}
	}
	return strings.Join(cleaned, ",")
}

const schema = `
CREATE TABLE IF NOT EXISTS shown (
  id            TEXT PRIMARY KEY,
  shown_on      TEXT NOT NULL,
  url           TEXT NOT NULL,
  image_url     TEXT NOT NULL,
  ratio         TEXT,
  width         INTEGER,
  height        INTEGER,
  tags          TEXT,
  favorited_at  TEXT
);
CREATE INDEX IF NOT EXISTS shown_favorited_idx ON shown (favorited_at);
`
