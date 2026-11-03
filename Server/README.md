# WeirdStreamer Server

The Go server behind WeirdStreamer. It indexes media folders, matches movies and TV shows on
TMDB, streams video (as-is or transcoded with ffmpeg), plays DVD ISOs, encodes offline downloads,
and serves the web UI. The Android TV and phone app talks to the same API.

## Requirements

- Go 1.24 with CGO (SQLite is `mattn/go-sqlite3`), so a C compiler too.
- `ffmpeg` and `ffprobe` on `PATH`, for thumbnails, trickplay, transcoding and disc playback.
- Python 3 with PyYAML (`python3-yaml`), for the scripts that read `config.yml`.
- Optional: a TMDB API read access token for movie and TV metadata.

## Build and run

Without Docker, `./start-server.sh` does everything:

- If `bin/` is missing, it runs `scripts/build.sh`, which builds `bin/server` and `bin/cli`.
- The first time, there's no `config.yml`, so it runs `scripts/create-config.sh`. That checks the
  tools (Go, gcc, ffmpeg, PyYAML, and openssl for a certificate), asks for the port, the libraries
  and whether to use HTTPS, generates a self-signed certificate if you want one, and writes
  `config.yml` from `config.example.yml` (with its comments) and a fresh JWT secret. It can also
  create the admin user.
- Then it runs `scripts/live.sh`, which keeps the server in this terminal and logs to
  `server.logsPath`.

You can also build with `make build` or `scripts/build.sh`, and create `config.yml` with
`scripts/create-config.sh` or by copying `config.example.yml` and editing it. The server reads
`./config.yml`, or the file named by `CONFIG_PATH`. It serves the web UI from `./web`, so run it from
this directory. Create the admin user with `./bin/cli user create <name> --admin --group parents`.

If `streaming.enableHTTP2` is on and the certificate and key files (`server.certFile` and
`server.keyFile`, default `cert.pem` and `key.pem`) exist, the server uses HTTPS (with HTTP/2).
Otherwise it falls back to plain HTTP.

## Release package

`./release.sh` at the top of the repository builds a release into `dist/<version>/`: a server
package (`weirdstreamer-server-<version>-linux-<arch>.tar.gz`) with the binaries, the web UI with
the Android app to download, `setup.sh` and `config.example.yml`, plus the APK and `SHA256SUMS`.

To install the package, unpack it and run `./setup.sh` in it. It asks for the server's name, port
and HTTPS and writes `config.yml`, can create the admin user, and can install a systemd service that
starts the server at boot (later: `./setup.sh service`; to undo it, `./setup.sh remove-service`).
Then it prints the web UI's address and offers to start the server and open it. Sign in as the
admin: while there are no libraries, Settings opens at Media Paths to add them (and the TMDB token).
Otherwise run `./bin/server` from that folder. It needs ffmpeg, nothing else.

## Docker

Files are in `docker/`. Copy `config.example.yml` to `config.yml` and edit it, then run
`make docker-compose-up`. That writes `docker/docker-compose.override.yml` (one read-only mount per
library, at the same path inside the container) and `docker/config.container.yml` (the config the
container runs with, its data under `var/`), then builds and starts the server on port 8080.

`make docker-config` asks which user the container runs as: you (the default, so it can read the
libraries and write `var/`), root, or another uid:gid. Pass `DOCKER_USER=current|root|uid:gid` to
choose without the prompt. Run it again after changing the libraries. For your own mounts (a folder outside the libraries, say),
copy `docker/docker-compose.extra.example.yml` to `docker/docker-compose.extra.yml` and add them there.
The generated files are rewritten, so don't edit them. The generated files are git-ignored,
and the container config holds the secrets. Create the admin user with
`docker compose -f docker/docker-compose.yml -f docker/docker-compose.override.yml exec weirdstreamer /app/cli user create admin --admin`.

## Configuration

See `config.example.yml`, which explains each setting in its comments. The main sections:

- `server`: `host`, `port`, and the TLS `certFile` and `keyFile` for HTTPS.
- `database`: paths for the users database (accounts, playback progress) and the media database
  (index, TMDB data, discs).
- `groups`: the access groups. Every user is in one of them, and each library lists those that
  may see it; nothing else may name a group. Settings → Groups (web) and the phone's Server tab
  add, rename (its users and libraries follow) and remove them (refused while still in use).
- `media.paths`: the libraries. Each has a `name`, a `path`, the `groups` allowed to see it, and
  an optional `type` of `movie` or `tv` (typed libraries get TMDB matching and the Library view).
  `continueWatchingMode` says where its videos in progress show on Continue Watching: `on` (the
  default: the home screen's list and the library's own), `libraryOnly` (only the library's own)
  or `off`. Settings → Media Paths edits it too.
- `media`: where thumbnails, trickplay frames, artwork and download encodes go, and which file
  extensions count as video, image and document.
- `auth`: the JWT secret and token lifetime. Change the secret.
- `metadata`: `tmdbAccessToken` and `language`.

Admins can also change libraries, groups and the TMDB token from the web UI's Settings. A change
there starts a metadata sync without a restart.

The server saves the file too (Settings, the Jobs page, `cli config add-path`). A save changes
only the values that changed: your comments, blank lines, the order of settings and anything the
server doesn't know stay as you wrote them.

`config.yml` holds secrets and is git-ignored.

## Users and access

Each user has one group, and sees a library only if that group is in the library's `groups`.

```bash
./bin/cli user create <name> [--admin] [--group <group>]
./bin/cli user list
./bin/cli user set-group <name> <group>
./bin/cli user change-password <name>
./bin/cli user delete <name>
```

The CLI can also scan libraries (`./bin/cli scan`) and generate trickplay frames ahead of time
(`./bin/cli trickplay [--video P | --dir P | --library NAME]`, `./bin/cli trickplay clean`).

## Code layout

- `src/cmd/server/main.go`: startup and all routes. `src/cmd/cli`: the admin CLI.
- `src/internal/handlers`: the HTTP API. Browse and stream, library and title pages, playback
  progress and Continue Watching, the admin Streams page, downloads, TMDB match fixing, users,
  config and access checks.
- `src/internal/media`: direct serving and ffmpeg transcoding, ISO9660/IFO parsing and disc
  streaming, download encodes, the active-stream tracker, thumbnails and trickplay.
- `src/internal/metadata`: TMDB matching and the library index.
- `src/internal/database`: SQLite. `src/internal/auth`: logins and JWTs.
- `web/`: the web UI (vanilla JS, no build step). Changes show up on a page reload.

## API overview

Everything under `/api` except login needs `Authorization: Bearer <token>` from
`POST /api/auth/login`. Paths are passed base64-encoded.

- `GET /api/media/browse`, `/api/media/stream`, `/api/media/details`: folders, streaming and
  file info. `stream` takes `transcode=1`, `start=`, `bitrate=`, `resolution=` and
  `auto-transcode=0`; discs also take `title`, `angle`, `audio` and `subtitle`.
- `GET /api/library`, `/api/library/title`: the Library view and title pages.
- `/api/playback/...`: progress sync, watched flags and Continue Watching.
- `/api/downloads...`: offline-download encodes.
- `/api/media/match...`: the Fix dialog (TMDB search, posters, episode mapping).
- `/api/config`, `/api/users`, `/api/admin/streams`: admin only.
- `/thumbnails/`, `/trickplay/`, `/artwork/`: images.
- `GET /health`, and `GET /local/streams` (active streams, from localhost only).

`src/api/openapi.yaml` is from an early version and doesn't cover most of this.

## Tests

```bash
cd src
go vet ./internal/... ./cmd/...
go test ./internal/...
```

Use `./internal/...`, not `./...`: the stray `main` files in `src/` (`test_auth.go`,
`test_login.go`) break the build.

`./scripts/run-e2e.sh` builds the server and runs a test instance from `e2e/` (git-ignored, with its own
config, accounts and sample media) on http://localhost:9100.

## License

Copyright (C) 2026 Ash Gandhi. Weird Streamer is free software under the GNU Affero General Public
License, version 3 or (at your option) any later version: see `LICENSE` and `NOTICE` at the top of
the repository. If you run a modified version for others over a network, the AGPL asks you to offer
them its source.
