#!/usr/bin/env bash
# Sets up a Weird Streamer release package (the folder this script is in): writes config.yml, can
# create the admin user, can install a service that starts the server at boot (systemd on Linux,
# launchd on macOS), and opens the web UI, where the admin adds the libraries (Settings opens there
# while there are none).
#
#   ./setup.sh                  all of it, asking as it goes
#   ./setup.sh service          install (or update) the service only
#   ./setup.sh remove-service   stop and remove the service
#   ./setup.sh gpu [--check]    set up the graphics card for hardware encoding (--check: only report)
#   ./setup.sh upgrade [--check] install the latest release from GitHub over this one (--check: only
#                               say whether there's a newer one); it backs up the databases first
#   ./setup.sh restore [FILE] [--with-config]
#                               put a backup's databases back (asks which; --with-config its
#                               config.yml too), stopping and restarting the service around it
#
# Without the service, run the server with ./bin/server from this folder.
set -euo pipefail
cd "$(dirname "$0")"
dir=$PWD
service=weirdstreamer
unit_file=/etc/systemd/system/$service.service
mac=0
[ "$(uname)" = Darwin ] && mac=1
label=com.weirdstreamer.server # macOS: the launchd daemon
plist_file=/Library/LaunchDaemons/$label.plist
if [ "$mac" = 1 ]; then
    # What the server runs (media.UseBundledTools): the package's own ffmpeg, then Homebrew's
    PATH="$dir/ffmpeg/bin:$PATH:/opt/homebrew/bin:/usr/local/bin"
    install_hint="brew install"
else
    install_hint="sudo apt install"
fi

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

# Installs Debian or Ubuntu packages (asks for sudo): when one isn't found, the package lists
# may be old, so they're updated and it's tried once more.
apt_install() {
    as_root apt-get install -y "$@" || { as_root apt-get update && as_root apt-get install -y "$@"; }
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

install_service() {
    if [ "$mac" = 1 ]; then
        install_launchd
        return
    fi
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
# cli server restart exits with status 3: a clean exit, started again
SuccessExitStatus=3
RestartForceExitStatus=3
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
    echo "Restart it (no sudo needed) with: bin/cli server restart"
}

# macOS: a launchd daemon in /Library/LaunchDaemons, run as this user, started at boot (before
# anyone logs in) and again if it stops on an error.
install_launchd() {
    [ -f config.yml ] || { echo "Write config.yml first: ./setup.sh" >&2; return 1; }
    local user group home
    user=$(id -un)
    group=$(id -gn)
    home=$(eval echo "~$user")
    mkdir -p var
    # Kept beside the server too, to see what was installed
    cat >"$label.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<!-- Weird Streamer, installed by $dir/setup.sh service -->
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$label</string>
    <key>UserName</key>
    <string>$user</string>
    <key>GroupName</key>
    <string>$group</string>
    <key>WorkingDirectory</key>
    <string>$dir</string>
    <key>ProgramArguments</key>
    <array>
        <string>$dir/bin/server</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>$home</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <!-- Started again if it stops on an error or to restart (cli server restart: status 3), not when it's stopped -->
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>ThrottleInterval</key>
    <integer>5</integer>
    <!-- It stops its ffmpeg runs and closes the databases on SIGTERM -->
    <key>ExitTimeOut</key>
    <integer>20</integer>
    <key>StandardOutPath</key>
    <string>$dir/var/server.out</string>
    <key>StandardErrorPath</key>
    <string>$dir/var/server.out</string>
</dict>
</plist>
PLIST
    plutil -lint "$label.plist" >/dev/null || { echo "The service's plist is malformed." >&2; return 1; }
    echo "== Installing $plist_file (asks for sudo)"
    as_root launchctl bootout "system/$label" 2>/dev/null || true
    as_root install -m 644 -o root -g wheel "$label.plist" "$plist_file"
    as_root launchctl enable "system/$label"
    as_root launchctl bootstrap system "$plist_file"
    sleep 2
    if as_root launchctl print "system/$label" 2>/dev/null | grep "state = running" >/dev/null; then
        echo "The server is running, and starts at boot."
    else
        echo "The service didn't start: see $dir/var/server.out" >&2
        return 1
    fi
    echo "Its output: $dir/var/server.out. Its auth and access logs are in config.yml's server.logsPath."
    echo "macOS refuses a service access to Desktop, Documents, Downloads, and removable and network"
    echo "volumes until it's allowed: if a library is in one of those, add $dir/bin/server to System"
    echo "Settings › Privacy & Security › Full Disk Access, then restart the service. Shares mounted at"
    echo "login (Finder) aren't there at boot: until they're mounted, the libraries on them are empty."
    service_running=1
    echo "Stop it with: sudo launchctl bootout system/$label. Remove it with: ./setup.sh remove-service"
    echo "Restart it (no sudo needed) with: bin/cli server restart"
}

remove_service() {
    if [ "$mac" = 1 ]; then
        if [ ! -f "$plist_file" ]; then
            echo "No $label service is installed."
            return 0
        fi
        echo "== Removing $plist_file (asks for sudo)"
        as_root launchctl bootout "system/$label" 2>/dev/null || true
        as_root rm -f "$plist_file"
        echo "Removed. The server no longer starts at boot."
        return 0
    fi
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

# Graphics cards for hardware encoding (Settings › Server › Streaming picks the encoder). Finds them
# by PCI vendor, which works before any driver is loaded, and lists what each needs: Intel and AMD
# encode through VA-API (Intel also Quick Sync), whose render node needs the video and render
# groups; NVIDIA needs its own driver loaded and its encode library. On Debian and Ubuntu it can
# install the packages (asking first); elsewhere it says what to install. A Mac needs nothing
# (VideoToolbox).
#   gpu_setup         ask, then make the changes
#   gpu_setup check   only report
gpu_setup() {
    local mode=${1:-ask} user vendors="" dev class vendor
    if [ "$(uname)" = Darwin ]; then
        echo "This Mac encodes with VideoToolbox (Apple Silicon): nothing to install. Choose it in Settings (Server › Streaming)."
        return 0
    fi
    user=$(id -un)
    for dev in "${PCI_DEVICES:-/sys/bus/pci/devices}"/*; do
        class=$(cat "$dev/class" 2>/dev/null) || continue
        [[ $class == 0x03* ]] || continue # display controllers
        vendor=$(cat "$dev/vendor" 2>/dev/null)
        case $vendor in
            0x8086) vendors+=" intel" ;;
            0x1002) vendors+=" amd" ;;
            0x10de) vendors+=" nvidia" ;;
        esac
    done
    if [ -z "$vendors" ]; then
        echo "No Intel, AMD or NVIDIA graphics found: transcodes and downloads use the CPU."
        return 0
    fi
    local apt=0 packages=() groups=() notes=() encoders="" name
    command -v apt-get >/dev/null && command -v dpkg-query >/dev/null && apt=1
    command -v ffmpeg >/dev/null && encoders=$(ffmpeg -hide_banner -encoders 2>/dev/null || true)
    if ! command -v ffmpeg >/dev/null; then
        # The server runs it for everything (hardware encoders are ffmpeg's): without it, none works.
        notes+=("ffmpeg isn't installed: the server needs it, hardware encoding too.")
        [ "$apt" = 1 ] && packages+=(ffmpeg)
    fi
    installed() { [[ $(dpkg-query -W -f='${Status}' "$1" 2>/dev/null || true) == *"ok installed" ]]; }
    available() { [ -n "$(apt-cache policy "$1" 2>/dev/null | sed -n 's/^ *Candidate: *//p' | grep -v '(none)')" ]; }
    want() { # want PACKAGE...: the first one apt has, unless one of them is installed
        local p
        for p in "$@"; do
            installed "$p" && return 0
            [[ " ${packages[*]} " == *" $p "* ]] && return 0
        done
        for p in "$@"; do available "$p" && { packages+=("$p"); return 0; }; done
        notes+=("Package $1 isn't available from apt's sources (Debian needs the non-free component, Ubuntu multiverse).")
    }
    want_groups() {
        local g
        for g in video render; do
            getent group "$g" >/dev/null || continue
            [ "$user" = root ] && continue
            [[ " $(id -nG "$user") " == *" $g "* ]] && continue
            [[ " ${groups[*]} " == *" $g "* ]] || groups+=("$g")
        done
    }
    for name in $(echo "$vendors" | tr ' ' '\n' | sort -u); do
        case $name in
            intel)
                echo "Found Intel graphics: VA-API and Quick Sync can encode."
                want_groups
                if [ "$apt" = 1 ]; then
                    want intel-media-va-driver-non-free intel-media-va-driver # VA-API (Broadwell and later)
                    [[ " ${packages[*]} " == *" intel-media-va-driver "* ]] &&
                        notes+=("Intel: only the free media driver is available; it can't encode on some chips (the non-free one can: Debian's non-free component).")
                    want libmfx-gen1.2 libmfx-gen1                            # Quick Sync's GPU runtime
                    want vainfo
                else
                    notes+=("Intel: install Intel's media driver (iHD) for VA-API, and Intel's VPL GPU runtime for Quick Sync.")
                fi
                [ -n "$encoders" ] && ! grep -q h264_vaapi <<<"$encoders" && notes+=("This ffmpeg has no VA-API encoder (h264_vaapi).")
                [ -n "$encoders" ] && ! grep -q h264_qsv <<<"$encoders" && notes+=("This ffmpeg has no Quick Sync encoder (h264_qsv).")
                ;;
            amd)
                echo "Found AMD graphics: VA-API can encode."
                want_groups
                if [ "$apt" = 1 ]; then
                    want mesa-va-drivers
                    want vainfo
                else
                    notes+=("AMD: install Mesa's VA-API drivers (Fedora's leave out H.264 encoding: RPM Fusion's mesa-va-drivers-freeworld has it).")
                fi
                [ -n "$encoders" ] && ! grep -q h264_vaapi <<<"$encoders" && notes+=("This ffmpeg has no VA-API encoder (h264_vaapi).")
                ;;
            nvidia)
                echo "Found NVIDIA graphics: NVENC can encode with NVIDIA's driver (not nouveau)."
                if command -v nvidia-smi >/dev/null && nvidia-smi >/dev/null 2>&1; then
                    local libs
                    libs=$(ldconfig -p 2>/dev/null || /sbin/ldconfig -p 2>/dev/null || true)
                    if ! grep -q 'libnvidia-encode\.so\.1' <<<"$libs"; then
                        local branch
                        branch=$(dpkg-query -W -f='${Package} ${Status}\n' 'nvidia-driver-*' 'nvidia-headless-*' 2>/dev/null |
                            awk '/ok installed/ {print $1}' | sed -n 's/^nvidia-\(driver\|headless\)-\([0-9]*\).*/\2/p' | head -1 || true)
                        if [ "$apt" = 1 ] && [ -n "$branch" ]; then
                            want "libnvidia-encode-$branch"
                        else
                            notes+=("NVIDIA: the driver is loaded but its encode library (libnvidia-encode) is missing.")
                        fi
                    fi
                elif command -v nvidia-smi >/dev/null; then
                    # Installed but not loaded: usually no module built for the running kernel
                    local driver
                    driver=$(dpkg-query -W -f='${Package} ${Status}\n' 'nvidia-driver-*' 2>/dev/null |
                        awk '/ok installed/ {print $1}' | sed -n 's/^nvidia-driver-//p' | head -1 || true)
                    if [ "$apt" = 1 ] && [ -n "$driver" ] && available "linux-modules-nvidia-$driver-$(uname -r)"; then
                        want "linux-modules-nvidia-$driver-$(uname -r)"
                        notes+=("NVIDIA: the driver's module wasn't built for this kernel ($(uname -r)); after installing it, a reboot (or sudo modprobe nvidia) loads it.")
                    else
                        notes+=("NVIDIA: the driver is installed but not loaded (nvidia-smi fails). Its kernel module may not be built for this kernel ($(uname -r)), or a reboot may be pending.")
                    fi
                elif command -v ubuntu-drivers >/dev/null; then
                    notes+=("NVIDIA: install NVIDIA's driver with: sudo ubuntu-drivers install (then reboot).")
                else
                    notes+=("NVIDIA: install NVIDIA's proprietary driver from your system's packages (then reboot).")
                fi
                [ -n "$encoders" ] && ! grep -q h264_nvenc <<<"$encoders" && notes+=("This ffmpeg has no NVENC encoder (h264_nvenc).")
                ;;
        esac
    done
    local n
    printf '%s\n' "${notes[@]}" | awk 'NF && !seen[$0]++ {print "  " $0}'
    if [ ${#packages[@]} = 0 ] && [ ${#groups[@]} = 0 ]; then
        [ ${#notes[@]} = 0 ] && echo "  Nothing to change: choose the encoder in Settings (Server › Streaming)."
        return 0
    fi
    [ ${#packages[@]} -gt 0 ] && echo "  Missing packages: ${packages[*]}"
    [ ${#groups[@]} -gt 0 ] && echo "  $user isn't in the group(s) the GPU needs: ${groups[*]}"
    [ "$mode" = check ] && return 0
    ask_yes "Install them and add $user to the groups, for hardware encoding (asks for sudo)?" y || return 0
    if [ ${#packages[@]} -gt 0 ]; then
        apt_install "${packages[@]}" || { echo "Installing the packages failed." >&2; return 1; }
    fi
    if [ ${#groups[@]} -gt 0 ]; then
        as_root usermod -aG "$(IFS=,; echo "${groups[*]}")" "$user"
        echo "Added $user to ${groups[*]}: a new login (and the service, which starts with them) picks that up."
    fi
    echo "The server finds the encoders when it starts: then choose one in Settings (Server › Streaming)."
}

# The releases upgrade takes, on GitHub (owner/name).
release_repo=${WEIRDSTREAMER_REPO:-vargonsoft/WeirdStreamer}

# This machine's package: linux-amd64, linux-arm64 or darwin-arm64.
platform() {
    local os arch
    case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) return 1 ;; esac
    case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) return 1 ;; esac
    [ "$os-$arch" = darwin-amd64 ] && return 1 # Intel Macs: no package
    echo "$os-$arch"
}

fetch() { # fetch URL FILE
    if command -v curl >/dev/null; then curl -fsSL -o "$2" "$1"
    else wget -q -O "$2" "$1"; fi
}

# The streams being watched now, from the running server (/local/streams, with the key it wrote
# beside config.yml): their count, or nothing when no server answers.
active_streams() {
    [ -f config.yml ] && [ -f local.key ] || return 0
    local port scheme out
    port=$(sed -n 's/^  port: *\([0-9]*\).*/\1/p' config.yml | head -1)
    scheme=http
    grep -qE '^  certFile: *"?[^" ]+' config.yml && scheme=https
    out=$(curl -sk --max-time 5 -H "X-Local-Key: $(cat local.key 2>/dev/null)" "$scheme://127.0.0.1:$port/local/streams" 2>/dev/null) || return 0
    sed -n 's/.*"active": *\([0-9]*\).*/\1/p' <<<"$out" | head -1
}

# Installs the latest release over this one: the program, the web UI and the scripts are replaced;
# config.yml, the data (var/), a built ffmpeg (ffmpeg/) and the service stay. The files replaced are
# kept in .previous/ (one upgrade back). A running service is restarted (asking first if someone
# is watching).
#   upgrade         install it
#   upgrade check   only say whether there's a newer one
upgrade() {
    local mode=${1:-install} target current latest tag url
    target=$(platform) || { echo "There's no release package for $(uname -s) on $(uname -m)." >&2; return 1; }
    command -v curl >/dev/null || command -v wget >/dev/null || { echo "Upgrading needs curl or wget." >&2; return 1; }
    current=$(cut -d' ' -f1 VERSION 2>/dev/null)
    # The latest release's tag, from where GitHub's /releases/latest redirects (no API, no rate limit).
    if command -v curl >/dev/null; then
        url=$(curl -fsSIL -o /dev/null -w '%{url_effective}' "https://github.com/$release_repo/releases/latest") || url=
    else
        url=$(wget -q --max-redirect=5 -S --spider "https://github.com/$release_repo/releases/latest" 2>&1 | sed -n 's/^ *Location: *//p' | tail -1)
    fi
    tag=${url##*/tag/}
    if [ -z "$url" ] || [ "$tag" = "$url" ]; then
        echo "Can't find the latest release at https://github.com/$release_repo/releases." >&2
        return 1
    fi
    latest=${tag#v}
    echo "This is ${current:-an unknown version}; the latest release is $latest."
    local base=${current%-dirty}
    if [ -n "$current" ] && { [ "$base" = "$latest" ] ||
        [ "$(printf '%s\n%s\n' "$base" "$latest" | sort -V | tail -1)" = "$base" ]; }; then
        echo "Nothing to upgrade."
        return 0
    fi
    [ "$mode" = check ] && { echo "./setup.sh upgrade installs it."; return 0; }
    ask_yes "Download and install $latest over this one?" y || return 0

    local work="$dir/.upgrade" name="weirdstreamer-server-$latest-$target.tar.gz"
    rm -rf "$work" && mkdir -p "$work" # here, so the files move into place by renaming
    echo "== Downloading $name"
    fetch "https://github.com/$release_repo/releases/download/$tag/$name" "$work/$name" &&
        fetch "https://github.com/$release_repo/releases/download/$tag/SHA256SUMS" "$work/SHA256SUMS" ||
        { echo "The download failed." >&2; rm -rf "$work"; return 1; }
    local want got
    want=$(awk -v f="$name" '$2 == f || $2 == "*"f {print $1}' "$work/SHA256SUMS")
    if command -v sha256sum >/dev/null; then got=$(sha256sum "$work/$name" | cut -d' ' -f1)
    else got=$(shasum -a 256 "$work/$name" | cut -d' ' -f1); fi
    if [ -z "$want" ] || [ "$want" != "$got" ]; then
        echo "The download doesn't match the release's checksum: not installing it." >&2
        rm -rf "$work"
        return 1
    fi
    tar -xzf "$work/$name" -C "$work" || { echo "Can't unpack the download." >&2; rm -rf "$work"; return 1; }
    local new
    new=$(find "$work" -mindepth 1 -maxdepth 1 -type d -name 'weirdstreamer-*' | head -1)
    [ -x "$new/bin/server" ] || { echo "The download has no bin/server." >&2; rm -rf "$work"; return 1; }

    # Asked before anything changes: a restart ends what's being watched.
    local restart=0 active
    if service_installed; then
        restart=1
        active=$(active_streams)
        if [ "${active:-0}" -gt 0 ] &&
            ! ask_yes "$active stream(s) are being watched; restarting the server ends them. Restart it now?" n; then
            restart=0
        fi
    fi

    # The databases and config first, in case the new version isn't wanted after all.
    echo "== Backing up the databases"
    ./bin/cli backup now || echo "No backup made (this version's cli can't); installing anyway."

    echo "== Installing $latest"
    rm -rf "$dir/.previous" && mkdir -p "$dir/.previous"
    local f
    for f in "$new"/* "$new"/.[!.]*; do
        [ -e "$f" ] || continue
        f=$(basename "$f")
        # The old one goes to .previous, the new one into its place: both renames (a running
        # program, this script included, keeps its old file).
        [ -e "$dir/$f" ] && mv "$dir/$f" "$dir/.previous/$f"
        mv "$new/$f" "$dir/$f"
    done
    rm -rf "$work"
    echo "Installed $latest (what it replaced is in .previous/). What's new: https://github.com/$release_repo/releases/tag/$tag"
    if [ "$restart" = 1 ]; then
        restart_service
    elif service_installed; then
        echo "The service is still running $current: restart it when no one is watching (./setup.sh service)."
    else
        echo "Restart the server (./bin/server) to run $latest."
    fi
}

# Whether this folder's server runs as a service (systemd or launchd).
service_installed() {
    if [ "$mac" = 1 ]; then [ -f "$plist_file" ] && grep -q "<string>$dir/bin/server</string>" "$plist_file"
    else [ -f "$unit_file" ] && grep -q "^ExecStart=\"$dir/bin/server\"" "$unit_file"; fi
}

# Puts a backup back (cli backup restore), with the server stopped: a service of this folder is
# stopped first (asking) and started again after.
restore() {
    local file="" with_config="" arg
    for arg in "$@"; do
        case $arg in --with-config) with_config=--with-config ;; *) file=$arg ;; esac
    done
    if [ -z "$file" ]; then
        local list
        list=$(./bin/cli backup list) || { echo "$list" >&2; return 1; }
        echo "$list"
        local names=() name # (no mapfile: macOS's bash 3.2)
        while read -r name; do names+=("$name"); done < <(sed -n 's/^  \(weirdstreamer-backup-[^ ]*\).*/\1/p' <<<"$list")
        [ ${#names[@]} -gt 0 ] || return 1
        local i
        for i in "${!names[@]}"; do echo "  $((i + 1)): ${names[$i]}"; done
        local pick
        pick=$(ask "Which one (1 is the newest)?" 1)
        [[ "$pick" =~ ^[0-9]+$ ]] && [ "$pick" -ge 1 ] && [ "$pick" -le ${#names[@]} ] ||
            { echo "No backup $pick." >&2; return 1; }
        file=${names[$((pick - 1))]}
    fi
    ask_yes "Put $file back${with_config:+ (with its config.yml)}? The databases now are kept aside." n || return 0
    local was_running=0
    if service_installed; then
        ask_yes "The service runs the server: stop it for the restore, and start it again after (asks for sudo)?" y || return 0
        was_running=1
        if [ "$mac" = 1 ]; then as_root launchctl bootout "system/$label" 2>/dev/null || true
        else as_root systemctl stop "$service"; fi
    fi
    local ok=0
    ./bin/cli backup restore "$file" $with_config || ok=1
    if [ "$was_running" = 1 ]; then
        echo "== Starting the server again"
        if [ "$mac" = 1 ]; then as_root launchctl bootstrap system "$plist_file"
        else as_root systemctl start "$service"; fi
    fi
    return $ok
}

restart_service() {
    echo "== Restarting the server (asks for sudo)"
    if [ "$mac" = 1 ]; then as_root launchctl kickstart -k "system/$label"
    else as_root systemctl restart "$service"; fi
    echo "The server restarted with the new version."
}

case "${1:-}" in
    service) install_service; exit ;;
    upgrade) if [ "${2:-}" = --check ]; then upgrade check; else upgrade; fi; exit ;;
    restore) shift; restore "$@"; exit ;;
    gpu) if [ "${2:-}" = --check ]; then gpu_setup check; else gpu_setup; fi; exit ;;
    remove-service) remove_service; exit ;;
    "") ;;
    -h|--help) sed -n '2,/^# Without the service/p' "$dir/$(basename "$0")"; exit 0 ;;
    *) echo "Unknown command: $1 (see --help)" >&2; exit 2 ;;
esac

# The CJK font the server looks for (media/cjkfont.go's places).
cjk_font_installed() {
    local f
    for f in /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc /usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc \
        /usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf \
        /usr/share/fonts/noto-cjk/NotoSansCJK-Regular.otf /usr/share/fonts/noto/NotoSansCJK-Regular.ttc; do
        [ -f "$f" ] && return 0
    done
    return 1
}

# What the server runs, besides itself: ffmpeg (required) and the tools for HEIC photos and CJK
# subtitles (optional). On Debian and Ubuntu, what's missing is offered for installing; elsewhere
# setup says what to install.
packages_setup() {
    local apt=0 optional=() why=()
    [ "$mac" = 0 ] && command -v apt-get >/dev/null && apt=1
    if ! command -v ffmpeg >/dev/null || ! command -v ffprobe >/dev/null; then
        echo "ffmpeg isn't installed: the server needs it for playing, thumbnails and everything else."
        if [ "$apt" = 1 ] && ask_yes "Install ffmpeg now (asks for sudo)?" y; then
            apt_install ffmpeg || echo "Installing ffmpeg failed: sudo apt install ffmpeg installs it." >&2
        fi
    fi
    if ! command -v ffmpeg >/dev/null || ! command -v ffprobe >/dev/null; then
        echo "Note: the server won't play anything until ffmpeg and ffprobe are installed:"
        echo "$install_hint ffmpeg (or your system's package)."
    elif ! ./check-ffmpeg.sh; then
        echo "Note: this ffmpeg lacks what's listed above (the README's ffmpeg section says how to get one"
        echo "that has it). Setup goes on; ./check-ffmpeg.sh checks again later."
    fi

    # Optional: HEIC/HEIF photos' thumbnails (and JPEG for browsers), and styled (ASS) subtitles in
    # Chinese, Japanese or Korean (a subset of a CJK font made by fontTools' pyftsubset).
    if ! command -v heif-convert >/dev/null; then
        if [ "$apt" = 1 ]; then
            optional+=(libheif-examples)
            why+=("libheif-examples: HEIC photos (iPhones' and many phones') get thumbnails and show in browsers")
        elif [ "$mac" = 1 ]; then
            echo "Note: heif-convert isn't installed, so HEIC photos will have no thumbnails and won't show in"
            echo "browsers: brew install libheif. Everything else works without it."
        else
            echo "Note: heif-convert isn't installed, so HEIC photos will have no thumbnails and won't show in"
            echo "browsers: install your system's libheif tools. Everything else works without it."
        fi
    fi
    local cjk=()
    command -v "${SUBSET:-pyftsubset}" >/dev/null || cjk+=(fonttools)
    cjk_font_installed || cjk+=(fonts-noto-cjk)
    if [ ${#cjk[@]} -gt 0 ]; then
        if [ "$apt" = 1 ]; then
            optional+=("${cjk[@]}")
            local size=""
            [[ " ${cjk[*]} " == *" fonts-noto-cjk "* ]] && size=" (the font is about 100 MB)"
            why+=("${cjk[*]}: styled subtitles in Chinese, Japanese or Korean show their characters, not boxes$size")
        else
            echo "Note: styled subtitles in Chinese, Japanese or Korean will show as boxes until fontTools"
            echo "(pyftsubset) and the Noto Sans CJK font are installed: see the README's CJK subtitle font section."
        fi
    fi
    if [ ${#optional[@]} -gt 0 ]; then
        echo "Optional, everything else works without them:"
        printf '  %s\n' "${why[@]}"
        if ask_yes "Install ${optional[*]} (asks for sudo)?" y; then
            apt_install "${optional[@]}" || echo "Installing them failed: sudo apt install ${optional[*]} installs them." >&2
        fi
    fi
}

# macOS: Homebrew's ffmpeg lacks the DVD reader, chromaprint and libass, so the package can build
# its own (./build-ffmpeg.sh, with Homebrew's libraries) into ffmpeg/, which the server uses first.
build_ffmpeg() {
    if ! command -v brew >/dev/null; then
        echo "Building ffmpeg needs Homebrew (https://brew.sh): install it, then run ./setup.sh again."
        return 1
    fi
    ask_yes "Build the server's own ffmpeg into $dir/ffmpeg now (about 10 minutes)?" y || return 1
    # Not setup's input: Homebrew and the build would read answers meant for setup's questions
    FFMPEG_PREFIX="$dir/ffmpeg" ./build-ffmpeg.sh </dev/null || return 1
    rm -rf "$dir/ffmpeg-build" # its sources and objects, only needed for building
}
if [ "$mac" = 1 ] && [ ! -x ffmpeg/bin/ffmpeg ] &&
    { ! command -v ffmpeg >/dev/null || ! ./check-ffmpeg.sh 2>/dev/null; }; then
    echo "== ffmpeg"
    echo "The server needs ffmpeg with x264, libass, the DVD reader and chromaprint. Homebrew's lacks"
    echo "the last three (styled subtitles, disc images, intro detection by sound)."
    build_ffmpeg || echo "Setup goes on; ./setup.sh again offers to build it."
fi
echo "== Programs the server uses"
packages_setup
echo "== Hardware encoding"
gpu_setup || true

if [ -f config.yml ]; then
    echo "config.yml already exists: leaving it. (Move it aside to set up again.)"
else
    write_config
fi

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
if [ "$mac" = 1 ]; then
    lan=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)
else
    lan=$(hostname -I 2>/dev/null | awk '{print $1}')
fi
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
if [ "$mac" = 1 ]; then
    if ask_yes "Install a launchd service, so the server starts at boot?" n; then
        install_service || true
    fi
elif command -v systemctl >/dev/null && [ -d /run/systemd/system ] &&
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
