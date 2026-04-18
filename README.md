# wallhaven-rotator

Daily wallpaper rotator for Hyprland backed by [Wallhaven](https://wallhaven.cc).
Rotates once per day from the hot feed (filtered by your config), lets you
favorite keepers into a saved collection, and swaps to a *favorites-only*
rotation mode when you're happy with what you have. Actions exposed as
Walker commands.

See [`DESIGN.md`](./DESIGN.md) for the architecture.

## Install

Requires Go 1.21+ and [`awww`](https://github.com/jcleira/awww) (or `swww` /
`hyprpaper`) running in your session.

```bash
go install github.com/jcleira/wallhaven-rotator/cmd/wallhaven-rotator@latest
```

On first run, a default config is written to
`$XDG_CONFIG_HOME/wallhaven-rotator/config.toml`.

## Commands

```
wallhaven-rotator next                 # fetch + apply a fresh wallpaper
wallhaven-rotator favorite             # save current to favorites/
wallhaven-rotator unfavorite           # remove current from favorites/
wallhaven-rotator mode <daily|favorites>
wallhaven-rotator info                 # JSON metadata for current
wallhaven-rotator open                 # open source page on wallhaven.cc
wallhaven-rotator status               # mode, last rotation, counts
wallhaven-rotator list-favorites       # list saved favorites
wallhaven-rotator daemon               # run from Hypr autostart; rotates once/day
wallhaven-rotator config-path          # print the config dir
```

## Config

All fields are optional — omit or leave empty to accept defaults.

```toml
[filter]
query      = ""                   # Wallhaven ?q=; empty = no text filter
categories = ["general"]          # general | anime | people
purity     = ["sfw"]              # v1 is SFW-locked
sorting    = "hot"                # hot | toplist | views | favorites | random
min_width  = 3840
min_height = 1600
ratios     = ["21x9", "32x9"]

[rotation]
mode                 = "daily"    # or "favorites"
delete_non_favorites = true
trigger              = "first-login"

[apply]
command             = "awww"      # awww | swww | hyprpaper
transition_type     = "grow"
transition_duration = "0.8"
transition_fps      = "60"
```

A symlink `~/.config/wallhaven-rotator/current.jpg` always points at the
displayed wallpaper. Point `hyprlock`'s background at that symlink and your
lockscreen will track your desktop automatically.

## Hyprland autostart

```
# ~/.config/hypr/conf/autostart.conf
exec-once = wallhaven-rotator daemon
```

The daemon is a silent no-op if already rotated today, so it's safe on every
login.

## Walker integration

Two ways to drive the rotator from Walker:

**1. Direct entries in the default launcher.** The [`walker/desktop/`](./walker/desktop/)
directory ships `.desktop` files for each action (`next`, `favorite`,
`unfavorite`, `mode daily`, `mode favorites`, `info`, `open`, `status`,
plus a top-level "Wallpaper" submenu entry). Symlink them into
`~/.local/share/applications/` and they'll appear when you type
"wallpaper" into Walker:

```bash
for f in ~/Code/wallhaven-rotator/walker/desktop/*.desktop; do
    ln -sf "$f" ~/.local/share/applications/
done
update-desktop-database ~/.local/share/applications
```

**2. Dedicated dmenu keybind.** [`walker/wallpaper-menu.sh`](./walker/wallpaper-menu.sh)
opens Walker in dmenu mode with just the rotator actions. Symlink it onto
your PATH and bind to a Hyprland shortcut:

```
# ~/.config/hypr/conf/keybinds.conf
bind = SUPER ALT,   W, exec, wallpaper-menu
bind = SUPER SHIFT, W, exec, wallhaven-rotator next
bind = SUPER CTRL,  W, exec, wallhaven-rotator favorite
```

Notifications (via `notify-send` / `mako`) surface results so you don't
need a terminal open.

## License

MIT — see [LICENSE](./LICENSE).
