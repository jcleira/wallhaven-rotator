// Package favorites manages the saved wallpaper collection on disk.
package favorites

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Meta is the sidecar JSON stored next to each favorited image.
type Meta struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	ImageURL   string   `json:"image_url"`
	Ratio      string   `json:"ratio"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Tags       []string `json:"tags"`
	FavoritedAt string  `json:"favorited_at"`
}

// Add copies src into dir as <id>.<ext>, writing a sidecar <id>.meta.json.
// Returns the destination image path.
func Add(dir, id string, src string, meta Meta) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(src)
	if ext == "" {
		ext = ".jpg"
	}
	dst := filepath.Join(dir, id+ext)
	if err := copyFile(src, dst); err != nil {
		return "", err
	}
	metaPath := filepath.Join(dir, id+".meta.json")
	f, err := os.Create(metaPath)
	if err != nil {
		return dst, err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(meta); err != nil {
		return dst, err
	}
	return dst, nil
}

// Remove deletes the favorite image + sidecar by id.
func Remove(dir, id string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var removed bool
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, id+".") || name == id {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return err
			}
			removed = true
		}
	}
	if !removed {
		return fmt.Errorf("no favorite with id %q in %s", id, dir)
	}
	return nil
}

// List returns all favorite image paths, sorted by filename (stable).
// Excludes sidecar .meta.json files.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".meta.json") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out, nil
}

// LoadMeta reads the sidecar for an image path, if it exists.
func LoadMeta(imagePath string) (Meta, error) {
	id := strings.TrimSuffix(filepath.Base(imagePath), filepath.Ext(imagePath))
	metaPath := filepath.Join(filepath.Dir(imagePath), id+".meta.json")
	f, err := os.Open(metaPath)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	var m Meta
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()
	if _, err := io.Copy(df, sf); err != nil {
		return err
	}
	return nil
}
