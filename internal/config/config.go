// Package config loads and persists the user config + runtime state.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the user-editable config at $XDG_CONFIG_HOME/wallhaven-rotator/config.toml.
type Config struct {
	Filter   Filter   `toml:"filter"`
	Rotation Rotation `toml:"rotation"`
	Paths    Paths    `toml:"paths"`
	Apply    Apply    `toml:"apply"`
}

type Filter struct {
	Query      string   `toml:"query"`
	Tags       []string `toml:"tags"`
	Categories []string `toml:"categories"` // general | anime | people
	Purity     []string `toml:"purity"`     // sfw only in v1
	Sorting    string   `toml:"sorting"`    // hot | toplist | ...
	MinWidth   int      `toml:"min_width"`
	MinHeight  int      `toml:"min_height"`
	Ratios     []string `toml:"ratios"`
	Colors     []string `toml:"colors"`
}

type Rotation struct {
	Mode               string `toml:"mode"` // daily | favorites
	DeleteNonFavorites bool   `toml:"delete_non_favorites"`
	Trigger            string `toml:"trigger"`    // first-login | timer
	TimerTime          string `toml:"timer_time"` // HH:MM
}

type Paths struct {
	ConfigDir    string `toml:"config_dir"`
	FavoritesDir string `toml:"favorites_dir"`
	CurrentLink  string `toml:"current_link"`
}

type Apply struct {
	Command            string `toml:"command"`
	TransitionType     string `toml:"transition_type"`
	TransitionDuration string `toml:"transition_duration"`
	TransitionFPS      string `toml:"transition_fps"`
}

// State is ephemeral runtime state tracked in state.toml.
type State struct {
	Mode              string `toml:"mode"`
	LastRotatedDate   string `toml:"last_rotated_date"` // YYYY-MM-DD
	CurrentID         string `toml:"current_id"`
	CurrentWallpaper  string `toml:"current_wallpaper"` // absolute path to the displayed file
	FavoritesIndex    int    `toml:"favorites_index"`   // position when rotating favorites
}

// Default returns a config with sensible v1 defaults.
func Default() Config {
	return Config{
		Filter: Filter{
			Query:      "",
			Categories: []string{"general"},
			Purity:     []string{"sfw"},
			Sorting:    "hot",
			MinWidth:   3840,
			MinHeight:  1600,
			Ratios:     []string{"21x9", "32x9"},
		},
		Rotation: Rotation{
			Mode:               "daily",
			DeleteNonFavorites: true,
			Trigger:            "first-login",
			TimerTime:          "07:00",
		},
		Apply: Apply{
			Command:            "awww",
			TransitionType:     "grow",
			TransitionDuration: "0.8",
			TransitionFPS:      "60",
		},
	}
}

// Dir returns the resolved config directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "wallhaven-rotator")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func configPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

func statePath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "state.toml"), nil
}

// Load reads config.toml, writing defaults on first run.
func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		def := Default()
		if err := Save(def); err != nil {
			return Config{}, fmt.Errorf("write default config: %w", err)
		}
		return def, nil
	}
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return Config{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return c, nil
}

// Save writes the config to config.toml.
func Save(c Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(configHeader); err != nil {
		return err
	}
	return toml.NewEncoder(f).Encode(c)
}

// LoadState reads state.toml, returning a zero value if it doesn't exist.
func LoadState() (State, error) {
	path, err := statePath()
	if err != nil {
		return State{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return State{Mode: "daily"}, nil
	}
	var s State
	if _, err := toml.DecodeFile(path, &s); err != nil {
		return State{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return s, nil
}

func SaveState(s State) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(s)
}

// ResolvedCurrentLink returns the configured symlink path, or the default.
func (c Config) ResolvedCurrentLink() (string, error) {
	if c.Paths.CurrentLink != "" {
		return expand(c.Paths.CurrentLink), nil
	}
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "current.jpg"), nil
}

// ResolvedFavoritesDir returns the configured favorites directory, or the default.
func (c Config) ResolvedFavoritesDir() (string, error) {
	if c.Paths.FavoritesDir != "" {
		return expand(c.Paths.FavoritesDir), nil
	}
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "favorites"), nil
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}

const configHeader = `# wallhaven-rotator configuration.
# See https://github.com/jcleira/wallhaven-rotator for docs.

`
