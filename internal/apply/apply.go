// Package apply wraps the wallpaper-setting command (awww / swww / hyprpaper).
package apply

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/jcleira/wallhaven-rotator/internal/config"
)

// Set applies imagePath as the current wallpaper using the configured command.
func Set(cfg config.Apply, imagePath string) error {
	switch cfg.Command {
	case "awww", "":
		return setAwww(cfg, imagePath)
	case "swww":
		return setSwww(cfg, imagePath)
	case "hyprpaper":
		return setHyprpaper(imagePath)
	default:
		return fmt.Errorf("unknown apply command %q (expected awww|swww|hyprpaper)", cfg.Command)
	}
}

// WaitReady polls the wallpaper backend until it answers a query or the
// context/deadline ends. Autostart fires awww-daemon and this rotator in
// parallel, so the rotator has to wait for the socket before it can apply.
func WaitReady(ctx context.Context, cfg config.Apply) error {
	bin := cfg.Command
	if bin == "" {
		bin = "awww"
	}
	if bin == "hyprpaper" {
		return nil
	}
	deadline := time.Now().Add(15 * time.Second)
	const pause = 250 * time.Millisecond
	var lastErr error
	for time.Now().Before(deadline) {
		if err := exec.CommandContext(ctx, bin, "query").Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pause):
		}
	}
	return fmt.Errorf("backend %q not ready within 15s: %v", bin, lastErr)
}

func setAwww(cfg config.Apply, p string) error {
	args := []string{"img", p}
	if cfg.TransitionType != "" {
		args = append(args, "--transition-type", cfg.TransitionType)
	}
	if cfg.TransitionDuration != "" {
		args = append(args, "--transition-duration", cfg.TransitionDuration)
	}
	if cfg.TransitionFPS != "" {
		args = append(args, "--transition-fps", cfg.TransitionFPS)
	}
	return runCmd("awww", args...)
}

func setSwww(cfg config.Apply, p string) error {
	args := []string{"img", p}
	if cfg.TransitionType != "" {
		args = append(args, "--transition-type", cfg.TransitionType)
	}
	if cfg.TransitionDuration != "" {
		args = append(args, "--transition-duration", cfg.TransitionDuration)
	}
	if cfg.TransitionFPS != "" {
		args = append(args, "--transition-fps", cfg.TransitionFPS)
	}
	return runCmd("swww", args...)
}

func setHyprpaper(p string) error {
	if err := runCmd("hyprctl", "hyprpaper", "preload", p); err != nil {
		return err
	}
	return runCmd("hyprctl", "hyprpaper", "wallpaper", ",$"+p)
}

func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("%s %v failed: %s", name, args, exitErr.Stderr)
		}
		return fmt.Errorf("%s %v: %w (%s)", name, args, err, out)
	}
	return nil
}
