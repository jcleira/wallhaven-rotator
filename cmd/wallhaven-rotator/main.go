// Command wallhaven-rotator is a daily wallpaper rotator for Hyprland.
//
// See DESIGN.md for architecture and the README for install steps.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jcleira/wallhaven-rotator/internal/config"
	"github.com/jcleira/wallhaven-rotator/internal/favorites"
	"github.com/jcleira/wallhaven-rotator/internal/history"
	"github.com/jcleira/wallhaven-rotator/internal/rotate"
)

const usage = `wallhaven-rotator — daily Wallhaven wallpaper rotator.

Usage: wallhaven-rotator <command> [args]

Commands:
  next                   Fetch + apply a new wallpaper now.
  favorite [id]          Mark current (or given id) as favorite.
  unfavorite [id]        Unfavorite current (or given id).
  mode <daily|favorites> Switch rotation mode.
  info                   Show current wallpaper metadata.
  open                   Open current wallpaper's Wallhaven page in browser.
  status                 Show mode, last rotation, counts.
  daemon                 First-login-of-day check (called from hypr autostart).
  list-favorites         List saved favorites.
  config-path            Print the config directory.
  version                Print version.
`

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cmd := os.Args[1]
	args := os.Args[2:]

	if err := dispatch(ctx, cmd, args); err != nil {
		fmt.Fprintf(os.Stderr, "wallhaven-rotator: %v\n", err)
		os.Exit(1)
	}
}

func dispatch(ctx context.Context, cmd string, args []string) error {
	switch cmd {
	case "next":
		return cmdNext(ctx)
	case "favorite", "fav":
		return cmdFavorite(args)
	case "unfavorite", "unfav":
		return cmdUnfavorite(args)
	case "mode":
		return cmdMode(args)
	case "info":
		return cmdInfo()
	case "open":
		return cmdOpen()
	case "status":
		return cmdStatus()
	case "daemon":
		return cmdDaemon(ctx)
	case "list-favorites", "favorites":
		return cmdListFavorites()
	case "config-path":
		return cmdConfigPath()
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q; run `wallhaven-rotator help`", cmd)
	}
}

func cmdNext(ctx context.Context) error {
	cfg, state, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	id, path, err := rotate.Next(ctx, cfg, state, store)
	if err != nil {
		return err
	}
	fmt.Printf("applied %s\n%s\n", id, path)
	return nil
}

func cmdFavorite(args []string) error {
	cfg, state, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	id := state.CurrentID
	if len(args) > 0 && args[0] != "" {
		id = args[0]
	}
	if id == "" {
		return fmt.Errorf("no current wallpaper; run `wallhaven-rotator next` first")
	}
	entry, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("get %s from history: %w", id, err)
	}
	if entry.FavoritedAt != "" {
		fmt.Printf("%s already favorited at %s\n", id, entry.FavoritedAt)
		return nil
	}

	favDir, err := cfg.ResolvedFavoritesDir()
	if err != nil {
		return err
	}
	src := state.CurrentWallpaper
	if src == "" || !fileExists(src) {
		return fmt.Errorf("current wallpaper file not found on disk: %q", src)
	}
	tags := strings.Split(entry.Tags, ",")
	_, err = favorites.Add(favDir, id, src, favorites.Meta{
		ID:          id,
		URL:         entry.URL,
		ImageURL:    entry.ImageURL,
		Ratio:       entry.Ratio,
		Width:       entry.Width,
		Height:      entry.Height,
		Tags:        tags,
		FavoritedAt: time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	if err := store.MarkFavorited(id); err != nil {
		return err
	}
	fmt.Printf("favorited %s → %s\n", id, favDir)
	return nil
}

func cmdUnfavorite(args []string) error {
	cfg, state, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	id := state.CurrentID
	if len(args) > 0 && args[0] != "" {
		id = args[0]
	}
	if id == "" {
		return fmt.Errorf("no current wallpaper; pass an id")
	}
	favDir, err := cfg.ResolvedFavoritesDir()
	if err != nil {
		return err
	}
	if err := favorites.Remove(favDir, id); err != nil {
		return err
	}
	if err := store.MarkUnfavorited(id); err != nil {
		return err
	}
	fmt.Printf("unfavorited %s\n", id)
	return nil
}

func cmdMode(args []string) error {
	if len(args) == 0 {
		state, err := config.LoadState()
		if err != nil {
			return err
		}
		fmt.Println(state.Mode)
		return nil
	}
	m := strings.ToLower(args[0])
	if m != "daily" && m != "favorites" {
		return fmt.Errorf("unknown mode %q (expected daily|favorites)", m)
	}
	state, err := config.LoadState()
	if err != nil {
		return err
	}
	state.Mode = m
	if err := config.SaveState(state); err != nil {
		return err
	}
	fmt.Printf("mode → %s\n", m)
	return nil
}

func cmdInfo() error {
	cfg, state, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	if state.CurrentID == "" {
		fmt.Println("no current wallpaper")
		return nil
	}

	out := map[string]any{
		"id":               state.CurrentID,
		"file":             state.CurrentWallpaper,
		"mode":             state.Mode,
		"last_rotated":     state.LastRotatedDate,
	}

	// Try history first, then favorites sidecar.
	if e, err := store.Get(state.CurrentID); err == nil {
		out["url"] = e.URL
		out["image_url"] = e.ImageURL
		out["ratio"] = e.Ratio
		out["width"] = e.Width
		out["height"] = e.Height
		out["tags"] = e.Tags
		out["favorited_at"] = e.FavoritedAt
	} else {
		if m, err := favorites.LoadMeta(state.CurrentWallpaper); err == nil {
			out["url"] = m.URL
			out["image_url"] = m.ImageURL
			out["ratio"] = m.Ratio
			out["width"] = m.Width
			out["height"] = m.Height
			out["tags"] = strings.Join(m.Tags, ",")
			out["favorited_at"] = m.FavoritedAt
		}
	}

	_ = cfg // reserved; might show filter/config in future
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func cmdOpen() error {
	_, state, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	if state.CurrentID == "" {
		return fmt.Errorf("no current wallpaper")
	}
	e, err := store.Get(state.CurrentID)
	if err == nil && e.URL != "" {
		return openURL(e.URL)
	}
	return openURL("https://wallhaven.cc/w/" + state.CurrentID)
}

func cmdStatus() error {
	cfg, state, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	total, _ := store.Count()
	favCount, _ := store.FavoriteCount()

	dir, _ := config.Dir()
	link, _ := cfg.ResolvedCurrentLink()
	favDir, _ := cfg.ResolvedFavoritesDir()

	fmt.Printf("mode          %s\n", emptyOr(state.Mode, "daily"))
	fmt.Printf("last rotated  %s\n", emptyOr(state.LastRotatedDate, "(never)"))
	fmt.Printf("current id    %s\n", emptyOr(state.CurrentID, "(none)"))
	fmt.Printf("current file  %s\n", emptyOr(state.CurrentWallpaper, "(none)"))
	fmt.Printf("history       %d wallpapers\n", total)
	fmt.Printf("favorites     %d\n", favCount)
	fmt.Printf("config dir    %s\n", dir)
	fmt.Printf("current link  %s\n", link)
	fmt.Printf("favorites dir %s\n", favDir)
	return nil
}

func cmdDaemon(ctx context.Context) error {
	state, err := config.LoadState()
	if err != nil {
		return err
	}
	today := time.Now().Format("2006-01-02")
	if state.LastRotatedDate == today {
		return nil // already rotated today, silent no-op
	}
	// Delegate to the normal next path, but swallow network errors so
	// offline boots don't fail autostart.
	if err := cmdNext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "wallhaven-rotator daemon: rotation skipped: %v\n", err)
		return nil
	}
	return nil
}

func cmdListFavorites() error {
	cfg, _, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	favDir, err := cfg.ResolvedFavoritesDir()
	if err != nil {
		return err
	}
	list, err := favorites.List(favDir)
	if err != nil {
		return err
	}
	sort.Strings(list)
	for _, p := range list {
		fmt.Println(p)
	}
	return nil
}

func cmdConfigPath() error {
	d, err := config.Dir()
	if err != nil {
		return err
	}
	fmt.Println(d)
	return nil
}

// bootstrap loads config + state + history store, ensuring the config
// directory exists.
func bootstrap() (config.Config, *config.State, *history.Store, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, nil, fmt.Errorf("load config: %w", err)
	}
	state, err := config.LoadState()
	if err != nil {
		return config.Config{}, nil, nil, fmt.Errorf("load state: %w", err)
	}
	dir, err := config.Dir()
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	store, err := history.Open(dir)
	if err != nil {
		return config.Config{}, nil, nil, fmt.Errorf("open history: %w", err)
	}
	return cfg, &state, store, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func emptyOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func openURL(u string) error {
	// Best-effort; xdg-open on Linux.
	if _, err := exec.LookPath("xdg-open"); err == nil {
		return exec.Command("xdg-open", u).Start()
	}
	fmt.Println(u)
	return nil
}

