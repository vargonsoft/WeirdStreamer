#!/usr/bin/env bash
# Guided creation of config.json from config.example.json, for running without Docker: asks for the
# port, libraries and HTTPS settings, makes a self-signed certificate if you want one, and creates an
# admin user. Run from anywhere. start-server.sh runs it when there's no config.json.
set -euo pipefail
cd "$(dirname "$0")/.."

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

if [ -f config.json ]; then
    echo "config.json already exists. Edit it, or move it aside to set up again."
    exit 0
fi

echo "== Checking what the server needs"
missing=()
command -v gcc >/dev/null || missing+=(gcc)
command -v ffmpeg >/dev/null || missing+=(ffmpeg)
if [ "${#missing[@]}" -gt 0 ]; then
    echo "Not installed: ${missing[*]}. gcc is needed to build; ffmpeg for thumbnails and streaming."
    if command -v apt-get >/dev/null && ask_yes "Install them with apt (asks for sudo)?" y; then
        sudo apt-get install -y build-essential ffmpeg
    elif command -v brew >/dev/null && ask_yes "Install them with Homebrew?" y; then
        brew install ffmpeg
    else
        echo "Install ${missing[*]} and run this again." >&2
        exit 1
    fi
fi

if ! scripts/check-ffmpeg.sh; then
    echo "Install an ffmpeg with the features above, then run this again." >&2
    exit 1
fi

# go.mod asks for Go 1.24.4 or later
need_go=1
if command -v go >/dev/null; then
    installed=$(go env GOVERSION | sed 's/^go//')
    lowest=$(printf '%s\n%s\n' 1.24.4 "$installed" | sort -V | head -1)
    [ "$lowest" = 1.24.4 ] && need_go=0
fi
if [ "$need_go" = 1 ]; then
    echo "Go 1.24.4 or later is needed to build."
    if command -v brew >/dev/null && ask_yes "Install Go with Homebrew?" y; then
        brew install go
    else
        echo "Install Go from https://go.dev/dl/ and run this again." >&2
        exit 1
    fi
fi

echo
echo "== Settings"
port=$(ask "Port to listen on" 8080)
if ask_yes "Use HTTPS (with HTTP/2)?" n; then
    use_tls=1
    cert_dir=$(ask "Folder for the certificate and key" "var/config")
    mkdir -p "$cert_dir"
    if ask_yes "Generate a self-signed certificate and key there?" y; then
        if ! command -v openssl >/dev/null; then
            echo "openssl is needed to generate the certificate." >&2
            exit 1
        fi
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
else
    use_tls=0
fi

echo "Libraries: each is a folder of media. Leave the name empty when you're done."
libraries=()
while true; do
    name=$(ask "Library name" "")
    [ -n "$name" ] || break
    while true; do
        path=$(ask "Folder for $name (absolute)" "")
        [ -d "$path" ] && break
        echo "  $path is not a folder."
    done
    type=$(ask "Type for $name (movie, tv or none)" "none")
    case "$type" in movie|tv|none) ;; *) echo "  Unknown type; using none."; type=none ;; esac
    libraries+=("$name|$path|$type")
done

read -r -s -p "TMDB API read access token (optional, Enter to skip): " tmdb_token || true
echo

echo "== Writing config.json"
mkdir -p var
python3 - "$port" "$use_tls" "${cert_dir:-}" "${tmdb_token:-}" ${libraries[@]+"${libraries[@]}"} <<'PY'
import copy, json, secrets, sys
port, use_tls, cert_dir, tmdb = sys.argv[1], sys.argv[2] == "1", sys.argv[3], sys.argv[4]
libraries = sys.argv[5:]
config = json.load(open("config.example.json"))
config["server"]["port"] = int(port)
config["server"]["host"] = "0.0.0.0"
config["server"].pop("certFile", None)
config["server"].pop("keyFile", None)
if use_tls:
    config["server"]["certFile"] = f"{cert_dir}/cert.pem"
    config["server"]["keyFile"] = f"{cert_dir}/key.pem"
config["streaming"]["enableHTTP2"] = use_tls
config["auth"]["jwtSecret"] = secrets.token_hex(32)
config["metadata"]["tmdbAccessToken"] = tmdb
paths = []
for entry in libraries:
    name, path, kind = entry.split("|")
    library = {"name": name, "path": path, "groups": config["groups"]}
    if kind != "none":
        library["type"] = kind
    paths.append(library)
config["media"]["paths"] = paths
with open("config.json", "w") as f:
    json.dump(config, f, indent=2)
    f.write("\n")
PY
chmod 600 config.json
echo "Wrote config.json (keep it private: it holds the JWT secret)."

echo "== Admin user"
if ask_yes "Create an admin user now?" y; then
    admin=$(ask "Admin username" "admin")
    ./bin/cli user create "$admin" --admin --group parents
fi

echo
echo "config.json is ready. Build with scripts/build.sh, then run ./start-server.sh."
