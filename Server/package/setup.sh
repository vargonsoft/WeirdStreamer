#!/usr/bin/env bash
# Sets up a Weird Streamer release package (the folder this script is in): writes config.yml, can
# create the admin user, can install a systemd service that starts the server at boot, and opens
# the web UI, where the admin adds the libraries (Settings opens there while there are none).
#
#   ./setup.sh                  all of it, asking as it goes
#   ./setup.sh service          install (or update) the systemd service only
#   ./setup.sh remove-service   stop and remove the systemd service
#
# Without the service, run the server with ./bin/server from this folder.
set -euo pipefail
cd "$(dirname "$0")"
dir=$PWD
service=weirdstreamer
unit_file=/etc/systemd/system/$service.service

ask() { # ask PROMPT DEFAULT -> answer
    local answer
    read -r -p "$1 [$2]: " answer || true
    echo "${answer:-$2}"
}
ask_yes() { # ask_yes PROMPT DEFAULT(y|n) -> returns 0 for yes
    local answer
    read -r -p "$1 [$2]: " answer || true
    answer=${answer:-$2}
    [[ "$answer" =~ ^[Yy] ]]
}
# sudo, unless this already runs as root
as_root() {
    if [ "$(id -u)" = 0 ]; then "$@"; else sudo "$@"; fi
}

write_config() {
    echo "== Settings (written to config.yml; config.example.yml explains them all)"
    local name port options=()
    name=$(ask "Server name, which the apps show" "$(hostname)")
    port=$(ask "Port to listen on" 9000)
    options+=(-name "$name" -port "$port")
    if ask_yes "Use HTTPS (with HTTP/2)?" n; then
        local cert_dir
        cert_dir=$(ask "Folder for the certificate and key" "var/config")
        mkdir -p "$cert_dir"
        if [ -f "$cert_dir/cert.pem" ] && [ -f "$cert_dir/key.pem" ]; then
            echo "Using $cert_dir/cert.pem and $cert_dir/key.pem."
        elif ask_yes "Generate a self-signed certificate and key there?" y; then
            command -v openssl >/dev/null || { echo "openssl is needed to generate them." >&2; exit 1; }
            local host
            host=$(hostname)
            (
                umask 077
                openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -sha256 -days 365 \
                    -keyout "$cert_dir/key.pem" -out "$cert_dir/cert.pem" -subj "/CN=$host" \
                    -addext "subjectAltName=DNS:$host,DNS:localhost,IP:127.0.0.1" 2>/dev/null
            )
            echo "Wrote $cert_dir/cert.pem and $cert_dir/key.pem (valid for a year)."
        else
            echo "Put your certificate and key in $cert_dir as cert.pem and key.pem before starting."
        fi
        options+=(-cert "$cert_dir/cert.pem" -key "$cert_dir/key.pem")
    fi

    # Libraries, the TMDB token and the rest are set in the web UI's Settings, as the admin.
    ./bin/cli config init config.example.yml "${options[@]}"
    echo "config.yml is owner-only: it holds the JWT secret."
}

# A value of config.yml's media section (unquoted), or "" when it isn't set
media_setting() {
    sed -n "/^media:/,/^[^ #]/s/^  $1: *\"\{0,1\}\([^\"]*\)\"\{0,1\} *\$/\1/p" config.yml | head -1
}
# The system's package manager, for the offers below: apt-get or dnf, else ""
package_manager() {
    if command -v apt-get >/dev/null; then echo apt-get
    elif command -v dnf >/dev/null; then echo dnf
    fi
}
# Where the server looks for Noto Sans CJK by itself (cjkFontCandidates in internal/media/cjkfont.go)
cjk_font_candidates=(/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc /usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc
    /usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf
    /usr/share/fonts/noto-cjk/NotoSansCJK-Regular.otf /usr/share/fonts/noto/NotoSansCJK-Regular.ttc)
# Where Noto Sans CJK is: those places, else where fontconfig knows it. Prints nothing when there's none.
find_cjk_font() {
    local f
    for f in "${cjk_font_candidates[@]}"; do
        [ -f "$f" ] && { echo "$f"; return; }
    done
    command -v fc-list >/dev/null && fc-list --format '%{file}\n' 2>/dev/null | grep -m1 '/NotoSansCJK-Regular\.tt[co]$' || true
}

# Optional: styled (ASS) subtitles in Chinese, Japanese or Korean need a CJK font and fontTools'
# pyftsubset to make subsets of it (see the README's CJK subtitle font section). Offers to install
# what's missing, when there's someone to ask; nothing is installed or downloaded without a yes.
setup_cjk() {
    local subset font pm choice
    subset=$(media_setting subsetCommand)
    font=$(media_setting cjkFontPath)
    if [ -z "$font" ]; then
        font=$(find_cjk_font)
        # One the server wouldn't find by itself goes in config.yml
        [ -z "$font" ] || [[ " ${cjk_font_candidates[*]} " == *" $font "* ]] || ./bin/cli config set-cjk-font "$font"
    elif [ ! -f "$font" ]; then
        echo "Note: config.yml's media.cjkFontPath ($font) isn't there."
        font=$(find_cjk_font)
        [ -z "$font" ] || ./bin/cli config set-cjk-font "$font"
    fi
    if command -v "${subset:-pyftsubset}" >/dev/null && [ -n "$font" ]; then
        return 0
    fi
    echo "== Chinese, Japanese and Korean subtitles (optional)"
    echo "Styled subtitles in Chinese, Japanese or Korean need fontTools' pyftsubset and the Noto Sans CJK"
    echo "font, or those lines show as boxes (the README's CJK subtitle font section). Everything else"
    echo "works without them."
    if [ ! -t 0 ]; then
        command -v "${subset:-pyftsubset}" >/dev/null || echo "Note: pyftsubset (fontTools) isn't installed."
        [ -n "$font" ] || echo "Note: no Noto Sans CJK font found."
        return 0
    fi
    pm=$(package_manager)

    if ! command -v "${subset:-pyftsubset}" >/dev/null; then
        echo "pyftsubset isn't installed${subset:+ (config.yml names $subset)}. It can be installed:"
        [ -n "$pm" ] && echo "  s: the system's package (sudo $pm install fonttools)"
        command -v python3 >/dev/null &&
            echo "  v: in a Python virtualenv here ($dir/fonttools; pip downloads fonttools and brotli, about 10 MB, from PyPI)"
        echo "  n: not now"
        choice=$(ask "Install pyftsubset?" n)
        case "$choice" in
            [Ss]*)
                if [ -z "$pm" ]; then
                    echo "No apt-get or dnf here: install fontTools with your system's package manager."
                elif as_root "$pm" install fonttools && command -v pyftsubset >/dev/null; then
                    # The package's pyftsubset is on the PATH, the default
                    [ -z "$subset" ] || ./bin/cli config set-subset-command ""
                else
                    echo "fontTools wasn't installed. Setup goes on without it." >&2
                fi
                ;;
            [Vv]*)
                if python3 -m venv "$dir/fonttools" && "$dir/fonttools/bin/pip" install --quiet fonttools brotli &&
                    [ -x "$dir/fonttools/bin/pyftsubset" ]; then
                    ./bin/cli config set-subset-command "$dir/fonttools/bin/pyftsubset"
                else
                    echo "The virtualenv wasn't made (Debian and Ubuntu need the python3-venv package for it)." >&2
                    echo "Setup goes on without it." >&2
                fi
                ;;
            *) echo "Skipped. The README's CJK subtitle font section says how to add it later." ;;
        esac
    fi

    if [ -z "$font" ]; then
        local package="" size=""
        case "$pm" in
            apt-get)
                package=fonts-noto-cjk
                size=$(apt-cache show --no-all-versions "$package" 2>/dev/null |
                    awk '/^Size:/ { d = $2 } /^Installed-Size:/ { i = $2 } END { if (d && i) printf "a %d MB download, %d MB installed", (d + 524288) / 1048576, (i + 512) / 1024 }')
                ;;
            dnf) package=google-noto-sans-cjk-ttc-fonts ;;
        esac
        if [ -z "$package" ]; then
            echo "No Noto Sans CJK font found. Install it with your system's package manager (it's 20 MB or"
            echo "more), then set media.cjkFontPath in config.yml if the server doesn't find it."
        else
            [ -n "$size" ] || size="20 MB or more; $pm shows the size before installing"
            echo "No Noto Sans CJK font found. The system's $package package has it ($size)."
            if ask_yes "Install $package now (sudo $pm install $package)?" n; then
                if as_root "$pm" install "$package" && font=$(find_cjk_font) && [ -n "$font" ]; then
                    echo "Installed: $font"
                    # config.yml names it where the server wouldn't look by itself (or names one that's gone)
                    if [ -n "$(media_setting cjkFontPath)" ] || [[ " ${cjk_font_candidates[*]} " != *" $font "* ]]; then
                        ./bin/cli config set-cjk-font "$font"
                    fi
                else
                    echo "The font wasn't installed, or wasn't found after. Setup goes on without it." >&2
                fi
            else
                echo "Skipped. The README's CJK subtitle font section says how to add it later."
            fi
        fi
    fi
}

install_service() {
    if ! command -v systemctl >/dev/null || [ ! -d /run/systemd/system ]; then
        echo "This machine doesn't run systemd: start the server with ./bin/server instead." >&2
        return 1
    fi
    [ -f config.yml ] || { echo "Write config.yml first: ./setup.sh" >&2; return 1; }
    local user group port caps=""
    user=$(id -un)
    group=$(id -gn)
    if [ "$user" = root ]; then
        echo "Note: the service will run as root. Run setup as the user who should own the server instead."
    fi
    # Ports below 1024 need the right to bind them
    port=$(sed -n 's/^  port: *\([0-9]*\).*/\1/p' config.yml | head -1)
    if [ -n "$port" ] && [ "$port" -lt 1024 ]; then
        caps="AmbientCapabilities=CAP_NET_BIND_SERVICE"
    fi
    # Hardening that would hide or freeze the server's own folder is left out for it: it writes
    # its data there (var/).
    local hardening="NoNewPrivileges=true"
    case "$dir/" in /tmp/*|/var/tmp/*) ;; *) hardening+=$'\nPrivateTmp=true' ;; esac
    case "$dir/" in /usr/*|/etc/*|/boot/*|/efi/*) ;; *) hardening+=$'\nProtectSystem=full' ;; esac
    # Kept beside the server too, to see what was installed
    cat >"$service.service" <<EOF
# Weird Streamer, installed by $dir/setup.sh service
[Unit]
Description=Weird Streamer media server
Documentation=file://$dir/config.example.yml
Wants=network-online.target
# Libraries on network shares (SMB, NFS) are mounted first
After=network-online.target remote-fs.target

[Service]
Type=simple
User=$user
Group=$group
WorkingDirectory=$dir
ExecStart="$dir/bin/server"
# It stops its ffmpeg runs and closes the databases on SIGTERM
TimeoutStopSec=20
Restart=on-failure
RestartSec=5
$hardening
$caps

[Install]
WantedBy=multi-user.target
EOF
    echo "== Installing $unit_file (asks for sudo)"
    as_root install -m 644 "$service.service" "$unit_file"
    as_root systemctl daemon-reload
    as_root systemctl enable "$service"
    as_root systemctl restart "$service"
    sleep 2
    if systemctl is-active --quiet "$service"; then
        echo "The server is running, and starts at boot."
    else
        echo "The service didn't start: see journalctl -u $service" >&2
        return 1
    fi
    echo "Its output: journalctl -u $service -f. Its auth and access logs are in config.yml's server.logsPath."
    service_running=1
    echo "Stop it with: sudo systemctl stop $service. Remove it with: ./setup.sh remove-service"
}

remove_service() {
    if [ ! -f "$unit_file" ]; then
        echo "No $service service is installed."
        return 0
    fi
    echo "== Removing $unit_file (asks for sudo)"
    as_root systemctl disable --now "$service"
    as_root rm -f "$unit_file"
    as_root systemctl daemon-reload
    echo "Removed. The server no longer starts at boot."
}

case "${1:-}" in
    service) install_service; exit ;;
    remove-service) remove_service; exit ;;
    "") ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) echo "Unknown command: $1 (see --help)" >&2; exit 2 ;;
esac

if ! command -v ffmpeg >/dev/null || ! command -v ffprobe >/dev/null; then
    echo "Note: ffmpeg and ffprobe aren't installed. The server needs them for thumbnails and"
    echo "streaming: sudo apt install ffmpeg (or your system's package)."
elif ! ./check-ffmpeg.sh; then
    echo "Note: this ffmpeg lacks what's listed above (the README's ffmpeg section says how to get one"
    echo "that has it). Setup goes on; ./check-ffmpeg.sh checks again later."
fi
# Optional: only HEIC/HEIF photos need it (their thumbnails, and JPEG for browsers).
if ! command -v heif-convert >/dev/null; then
    echo "Note: heif-convert isn't installed, so HEIC photos (iPhones' and many phones') will have no"
    echo "thumbnails and won't show in browsers: sudo apt install libheif-examples (or your system's"
    echo "libheif tools). Everything else works without it."
fi

if [ -f config.yml ]; then
    echo "config.yml already exists: leaving it. (Move it aside to set up again.)"
else
    write_config
fi

setup_cjk

echo "== Admin user"
admin=""
if ask_yes "Create an admin user now?" y; then
    admin=$(ask "Admin username" "admin")
    ./bin/cli user create "$admin" --admin
fi

# The address to open: this machine's, and the one other devices on the network use.
port=$(sed -n 's/^  port: *\([0-9]*\).*/\1/p' config.yml | head -1)
scheme=http
grep -qE '^  certFile: *"?[^" ]+' config.yml && scheme=https
lan=$(hostname -I 2>/dev/null | awk '{print $1}')
local_url="$scheme://localhost:$port"

# Waits until the server answers on its port (up to 30 seconds).
wait_for_server() {
    for _ in $(seq 1 60); do
        (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null && return 0
        sleep 0.5
    done
    return 1
}
# Opens the web UI in a browser, where this machine has a desktop.
offer_browser() {
    local opener=""
    if [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ] && command -v xdg-open >/dev/null; then opener=xdg-open
    elif [ "$(uname)" = Darwin ]; then opener=open
    fi
    [ -n "$opener" ] || return 0
    if ask_yes "Open it in a browser now${admin:+ and sign in as $admin}?" y; then
        "$opener" "$local_url" >/dev/null 2>&1 &
    fi
}

echo "== Starting"
service_running=0
if command -v systemctl >/dev/null && [ -d /run/systemd/system ] &&
    ask_yes "Install a systemd service, so the server starts at boot?" n; then
    install_service || true
fi

echo
echo "The web UI: $local_url${lan:+ (from other devices: $scheme://$lan:$port)}"
[ "$scheme" = https ] && echo "With a self-signed certificate, the browser asks you to accept it first."
echo "Sign in as the admin user: Settings opens at Media Paths to add the libraries (and the TMDB token"
echo "for movie and TV libraries) while there are none."
if [ "$service_running" = 1 ]; then
    offer_browser
elif ask_yes "Start the server now, in this terminal (Ctrl-C stops it)?" y; then
    ./bin/server &
    server=$!
    if wait_for_server; then
        offer_browser
    else
        echo "The server hasn't answered on port $port: see its output above." >&2
    fi
    wait "$server"
else
    echo "Start it with ./bin/server (from this folder). ./setup.sh service sets it up to start at boot."
fi
