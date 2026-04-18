#!/usr/bin/env bash
# walker/wallpaper-menu.sh — wallhaven-rotator action menu for Walker.
#
# Invoked as a Hyprland keybind. Opens Walker in dmenu mode with a list of
# actions; dispatches to `wallhaven-rotator` based on the selection. Results
# (info, status) are surfaced via notify-send so the user sees them without
# a terminal.
#
# Dependencies: walker, wallhaven-rotator on PATH, notify-send (libnotify).

set -euo pipefail

BIN=${WALLHAVEN_ROTATOR:-wallhaven-rotator}
NOTIFY=${NOTIFY:-notify-send}
ICON=${WALLHAVEN_ICON:-image-x-generic}

notify() {
    if command -v "$NOTIFY" >/dev/null 2>&1; then
        "$NOTIFY" -i "$ICON" "$@"
    else
        printf '%s\n' "$@" >&2
    fi
}

mode_now=$("$BIN" mode 2>/dev/null || echo daily)

entries=(
    "next — fetch a new wallpaper now"
    "favorite — save current wallpaper"
    "unfavorite — remove current from favorites"
    "mode daily — rotate fresh Wallhaven pick"
    "mode favorites — rotate saved favorites only"
    "info — show current wallpaper metadata"
    "open — open source page on wallhaven.cc"
    "status — show rotation status"
    "list-favorites — list saved favorites"
)

selection=$(printf '%s\n' "${entries[@]}" \
    | walker --dmenu --placeholder "Wallpaper (mode: $mode_now)" --exit) || exit 0

action=${selection%% —*}

case "$action" in
    next)
        out=$("$BIN" next 2>&1) \
            && notify "Wallpaper" "Applied: $(echo "$out" | head -1)" \
            || notify "Wallpaper — failed" "$out"
        ;;
    favorite)
        out=$("$BIN" favorite 2>&1) && notify "Favorited" "$out" || notify "Favorite failed" "$out"
        ;;
    unfavorite)
        out=$("$BIN" unfavorite 2>&1) && notify "Unfavorited" "$out" || notify "Unfavorite failed" "$out"
        ;;
    "mode daily")
        "$BIN" mode daily >/dev/null && notify "Rotation mode" "daily"
        ;;
    "mode favorites")
        "$BIN" mode favorites >/dev/null && notify "Rotation mode" "favorites"
        ;;
    info)
        out=$("$BIN" info 2>&1 || true)
        notify "Wallpaper info" "$out"
        ;;
    open)
        "$BIN" open
        ;;
    status)
        out=$("$BIN" status 2>&1)
        notify "Wallpaper status" "$out"
        ;;
    list-favorites)
        out=$("$BIN" list-favorites 2>&1)
        notify "Favorites" "${out:-"(no favorites yet)"}"
        ;;
    *)
        exit 0
        ;;
esac
