// Package rotate implements the wallpaper-selection pipeline: pick a new
// wallpaper (or advance through favorites), download, apply, record.
package rotate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcleira/wallhaven-rotator/internal/apply"
	"github.com/jcleira/wallhaven-rotator/internal/config"
	"github.com/jcleira/wallhaven-rotator/internal/favorites"
	"github.com/jcleira/wallhaven-rotator/internal/history"
	"github.com/jcleira/wallhaven-rotator/internal/wallhaven"
)

const (
	maxSearchPages = 5
	searchTimeout  = 60 * time.Second
)

// Next advances the rotation by one — fetching a new wallpaper in daily mode
// or stepping through favorites in favorites mode. Returns the id and path
// of the newly-applied wallpaper.
func Next(ctx context.Context, cfg config.Config, state *config.State, store *history.Store) (id, path string, err error) {
	switch strings.ToLower(state.Mode) {
	case "", "daily":
		return nextDaily(ctx, cfg, state, store)
	case "favorites":
		return nextFavorites(ctx, cfg, state)
	default:
		return "", "", fmt.Errorf("unknown rotation mode %q", state.Mode)
	}
}

func nextDaily(ctx context.Context, cfg config.Config, state *config.State, store *history.Store) (string, string, error) {
	client := wallhaven.New()

	seen, err := store.SeenIDs()
	if err != nil {
		return "", "", fmt.Errorf("load history: %w", err)
	}

	params := buildSearchParams(cfg.Filter)
	w, err := pickUnseen(ctx, client, params, seen, maxSearchPages)
	if err != nil {
		// Widen: drop ratios, then min resolution, then fall back to toplist.
		if params2 := dropRatios(params); params2 != nil {
			if w, err2 := pickUnseen(ctx, client, *params2, seen, maxSearchPages); err2 == nil {
				return applyPicked(ctx, cfg, state, store, client, w)
			}
		}
		if params3 := dropResolution(params); params3 != nil {
			if w, err3 := pickUnseen(ctx, client, *params3, seen, maxSearchPages); err3 == nil {
				return applyPicked(ctx, cfg, state, store, client, w)
			}
		}
		params4 := params
		params4.Sorting = "toplist"
		if w, err4 := pickUnseen(ctx, client, params4, seen, maxSearchPages); err4 == nil {
			return applyPicked(ctx, cfg, state, store, client, w)
		}
		return "", "", fmt.Errorf("no unseen wallpaper matched filters: %w", err)
	}
	return applyPicked(ctx, cfg, state, store, client, w)
}

func applyPicked(ctx context.Context, cfg config.Config, state *config.State, store *history.Store, client *wallhaven.Client, w *wallhaven.Wallpaper) (string, string, error) {
	configDir, err := config.Dir()
	if err != nil {
		return "", "", err
	}
	cacheDir := filepath.Join(configDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", err
	}

	ext := guessExt(w.FileType, w.Path)
	dlPath := filepath.Join(cacheDir, w.ID+ext)
	if err := downloadTo(ctx, client, w.Path, dlPath); err != nil {
		return "", "", err
	}

	link, err := cfg.ResolvedCurrentLink()
	if err != nil {
		return "", "", err
	}

	previous := state.CurrentWallpaper
	previousID := state.CurrentID

	if err := updateSymlink(link, dlPath); err != nil {
		return "", "", err
	}
	if err := writeMetaSidecar(link, w); err != nil {
		// non-fatal
		fmt.Fprintf(os.Stderr, "warn: failed to write meta sidecar: %v\n", err)
	}

	if err := apply.Set(cfg.Apply, dlPath); err != nil {
		return "", "", fmt.Errorf("apply: %w", err)
	}

	if err := store.Record(history.Entry{
		ID:       w.ID,
		URL:      w.URL,
		ImageURL: w.Path,
		Ratio:    w.Ratio,
		Width:    w.DimensionX,
		Height:   w.DimensionY,
		ShownOn:  time.Now().Format("2006-01-02"),
	}); err != nil {
		return "", "", fmt.Errorf("record: %w", err)
	}

	state.CurrentID = w.ID
	state.CurrentWallpaper = dlPath
	state.LastRotatedDate = time.Now().Format("2006-01-02")
	if err := config.SaveState(*state); err != nil {
		return "", "", fmt.Errorf("save state: %w", err)
	}

	if cfg.Rotation.DeleteNonFavorites && previous != "" && previous != dlPath {
		if previousID == "" || !isFavorited(store, previousID) {
			_ = os.Remove(previous)
		}
	}

	return w.ID, dlPath, nil
}

func isFavorited(store *history.Store, id string) bool {
	e, err := store.Get(id)
	if err != nil {
		return false
	}
	return e.FavoritedAt != ""
}

func nextFavorites(ctx context.Context, cfg config.Config, state *config.State) (string, string, error) {
	favDir, err := cfg.ResolvedFavoritesDir()
	if err != nil {
		return "", "", err
	}
	list, err := favorites.List(favDir)
	if err != nil {
		return "", "", err
	}
	if len(list) == 0 {
		return "", "", fmt.Errorf("no favorites in %s — favorite some wallpapers first", favDir)
	}

	idx := (state.FavoritesIndex + 1) % len(list)
	pick := list[idx]

	link, err := cfg.ResolvedCurrentLink()
	if err != nil {
		return "", "", err
	}
	if err := updateSymlink(link, pick); err != nil {
		return "", "", err
	}
	if err := apply.Set(cfg.Apply, pick); err != nil {
		return "", "", fmt.Errorf("apply: %w", err)
	}

	id := strings.TrimSuffix(filepath.Base(pick), filepath.Ext(pick))
	state.CurrentID = id
	state.CurrentWallpaper = pick
	state.FavoritesIndex = idx
	state.LastRotatedDate = time.Now().Format("2006-01-02")
	if err := config.SaveState(*state); err != nil {
		return "", "", fmt.Errorf("save state: %w", err)
	}
	return id, pick, nil
}

func pickUnseen(parent context.Context, client *wallhaven.Client, params wallhaven.SearchParams, seen map[string]struct{}, maxPages int) (*wallhaven.Wallpaper, error) {
	ctx, cancel := context.WithTimeout(parent, searchTimeout)
	defer cancel()

	for page := 1; page <= maxPages; page++ {
		params.Page = page
		resp, err := client.Search(ctx, params)
		if err != nil {
			return nil, err
		}
		if len(resp.Data) == 0 {
			return nil, fmt.Errorf("no results on page %d", page)
		}
		for i := range resp.Data {
			w := &resp.Data[i]
			if _, already := seen[w.ID]; already {
				continue
			}
			return w, nil
		}
		if resp.Meta.CurrentPage >= resp.Meta.LastPage {
			break
		}
	}
	return nil, fmt.Errorf("no unseen results in first %d pages", maxPages)
}

func buildSearchParams(f config.Filter) wallhaven.SearchParams {
	p := wallhaven.SearchParams{
		Query:      f.Query,
		Categories: categoriesBitmap(f.Categories),
		Purity:     purityBitmap(f.Purity),
		Sorting:    defaultString(f.Sorting, "hot"),
		Ratios:     f.Ratios,
		Colors:     f.Colors,
	}
	if f.MinWidth > 0 && f.MinHeight > 0 {
		p.AtLeast = fmt.Sprintf("%dx%d", f.MinWidth, f.MinHeight)
	}
	return p
}

func dropRatios(p wallhaven.SearchParams) *wallhaven.SearchParams {
	if len(p.Ratios) == 0 {
		return nil
	}
	p.Ratios = nil
	return &p
}

func dropResolution(p wallhaven.SearchParams) *wallhaven.SearchParams {
	if p.AtLeast == "" {
		return nil
	}
	p.AtLeast = ""
	return &p
}

// categoriesBitmap converts ["general","anime"] to a Wallhaven 3-bit string
// like "110". Order is general|anime|people.
func categoriesBitmap(cats []string) string {
	bits := []rune{'0', '0', '0'}
	for _, c := range cats {
		switch strings.ToLower(strings.TrimSpace(c)) {
		case "general":
			bits[0] = '1'
		case "anime":
			bits[1] = '1'
		case "people":
			bits[2] = '1'
		}
	}
	s := string(bits)
	if s == "000" {
		return "100" // sensible fallback: general only
	}
	return s
}

// purityBitmap converts purity strings to Wallhaven's 3-bit mask (sfw|sketchy|nsfw).
// v1 locks this to SFW.
func purityBitmap(levels []string) string {
	bits := []rune{'0', '0', '0'}
	for _, l := range levels {
		switch strings.ToLower(strings.TrimSpace(l)) {
		case "sfw":
			bits[0] = '1'
		case "sketchy":
			bits[1] = '1'
		case "nsfw":
			bits[2] = '1'
		}
	}
	s := string(bits)
	if s == "000" {
		return "100"
	}
	return s
}

func defaultString(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func guessExt(mime, path string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	}
	if ext := filepath.Ext(path); ext != "" {
		return ext
	}
	return ".jpg"
}

func downloadTo(ctx context.Context, client *wallhaven.Client, url, dst string) error {
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := client.Download(ctx, url, f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// updateSymlink atomically points link → target.
func updateSymlink(link, target string) error {
	tmp := link + ".new"
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}

// writeMetaSidecar drops a JSON sidecar next to the current symlink describing
// what the symlink currently points at.
func writeMetaSidecar(link string, w *wallhaven.Wallpaper) error {
	sidecarPath := link + ".meta.json"
	f, err := os.Create(sidecarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, `{
  "id": %q,
  "url": %q,
  "image_url": %q,
  "ratio": %q,
  "width": %d,
  "height": %d
}
`, w.ID, w.URL, w.Path, w.Ratio, w.DimensionX, w.DimensionY)
	return err
}
