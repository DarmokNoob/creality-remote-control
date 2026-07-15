# Creality Halot One Remote Control

A self-hosted, local-only remote control web UI for the Creality Halot One
resin 3D printer — file browsing, print queue management, live camera feed,
and thumbnail previews, all running **directly on the printer itself**. No
cloud dependency, no companion app, no subscription.

Forked from [danielkucera/creality-remote-control](https://github.com/danielkucera/creality-remote-control),
which provided the original WebSocket protocol implementation for talking
to the printer's `PrinterUI` process (file upload, print start/pause/stop,
status polling). This fork rebuilds the surrounding application on top of
that protocol.

## Why this exists

The Halot One's official control options are Chitubox (subscription-based
slicer) or the Creality Cloud app (routes everything through Creality's
servers, including the camera feed). Neither is local-only or free of
external dependencies. This project replaces both with something that runs
entirely on your own network, using the printer's existing (but
undocumented) local WebSocket API and a small custom Go server for
everything the stock firmware doesn't expose over that API.

## Architecture

Everything runs on the printer itself (a rooted Halot One — see
[`docs/rooting-guide.md`](docs/rooting-guide.md) for how). Three processes,
three ports:

| Component | Port | What it does |
|---|---|---|
| `PrinterUI` (stock firmware) | 18188 | WebSocket API: file upload, print control, status. Not modified — this is Creality's own process. |
| `mjpg_streamer` (custom build, see `bin/`) | 8081 | Live MJPEG camera feed. Replaces Creality's stock `uvc_stream`, which crashes reliably under real client load — see [`docs/touch-injection-notes.md`](docs/touch-injection-notes.md) for the unrelated investigation that led to fully understanding that binary, and `printer-init/README.md` for why this replacement was necessary and how it's deployed. |
| `halot-server` (Go, this repo) | 8082 | Serves `index.html` and handles everything `PrinterUI`'s WebSocket API doesn't: browsing the USB drive's `.cxdlp` files, copying a selected file into the printer's internal print queue (`/mnt/UDISK`), and decoding/serving thumbnail previews directly from the raw `.cxdlp` file format. |

`index.html` is a single-page frontend that talks to `PrinterUI` (port
18188) directly via WebSocket for print control/status, and to
`halot-server` (port 8082, same origin) for file browsing and thumbnails.
It supports light/dark mode automatically and resolves all URLs relative to
whatever host actually served the page, so the same file works whether
you're hitting the printer directly by IP or hostname.

## Repository layout

```
main.go            — the Go server (build for the printer's actual
                      architecture: 32-bit ARM, EABI5 — see build notes below)
index.html          — the frontend, served by halot-server
bin/                — pre-built mjpg_streamer + its input_uvc/output_http
                      plugins (armhf, statically matched to the printer's
                      old glibc — see printer-init/README.md for why these
                      can't just be `apt install`ed)
printer-init/       — start_streamer.sh + deployment instructions for
                      getting mjpg_streamer running persistently on boot
docs/
  rooting-guide.md        — how root SSH access was obtained via ADB
  touch-injection-notes.md — investigation into remotely clearing a
                             stuck print-end screen (documented dead end,
                             kept for reference)
```

## Building halot-server

The printer runs a genuinely old environment: 32-bit ARM userspace despite
an aarch64 kernel, and glibc 2.23 (circa 2016). Go's static linking sidesteps
the version-matching problems this caused for the C-based `mjpg_streamer`
build entirely — just cross-compile directly, no special toolchain or
chroot needed:

```bash
GOOS=linux GOARCH=arm GOARM=7 go build -o halot-server main.go
```

## Deploying

```bash
scp halot-server root@<printer-ip>:/mnt/UDISK/
scp index.html root@<printer-ip>:/mnt/UDISK/www-ui/
```

`halot-server` needs to be running (currently started manually — a proper
boot-time init hook for it, matching the pattern in `printer-init/`, is a
reasonable next step). Once running, the whole UI is available at
`http://<printer-ip>:8082`.

For the camera feed setup, see [`printer-init/README.md`](printer-init/README.md).

## Known limitations

- A finished/stopped print (`PRINT_END` status) can currently only be
  cleared by physically tapping the printer's touchscreen, or a full power
  cycle — there's no remote command for it. Thoroughly investigated in
  [`docs/touch-injection-notes.md`](docs/touch-injection-notes.md); not a
  quick fix.
- `halot-server` isn't yet wired into a boot-time init hook the way
  `mjpg_streamer` is — needs to be started manually after a reboot for now.
