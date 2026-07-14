# Printer Init Scripts

These files belong on the Halot One itself, not the Pi. They're version-controlled
here for reference and easy redeployment after a firmware update wipes `/overlay`
(which resets `/etc/rc.local` and anything else outside `/mnt/UDISK`).

## What's here

- `start_streamer.sh` — waits for the camera device to exist, then launches our
  custom `mjpg_streamer` build (see `bin/` in the repo root) on port 8081.
  Logs to `/mnt/UDISK/mjpg_streamer_boot.log`.

## Deployment

### 1. Disable Creality's stock camera service

Creality's own `uvc_stream` binary (launched by `S80camera`) is unstable — it
segfaults reliably within about a minute under any real client load (see
`docs/camera-investigation-notes.md` if that exists, or just: we spent a full
session proving this via QEMU/chroot cross-compilation and disassembly). It also
holds `/dev/video0` exclusively, so it must be disabled for our streamer to get
the camera at all.

```bash
mv /etc/rc.d/S80camera /etc/rc.d/.S80camera
```

Renaming (not deleting) means it's easy to restore if needed, and the leading
`.` makes the init system's `S*`/`K*` glob pattern skip it.

Kill the currently-running instance too (only needed once, since it won't
restart after the rename):

```bash
kill $(ps | grep uvc_stream | grep -v grep | awk '{print $1}')
```

### 2. Copy the streamer binary, plugins, and web assets

From the repo root, on a machine with SSH access to the printer:

```bash
scp bin/mjpg_streamer bin/input_uvc.so bin/output_http.so root@10.168.113.245:/mnt/UDISK/
```

The `www/` folder (mjpg-streamer's built-in viewer page) also needs to exist at
`/mnt/UDISK/www/` — it's not checked into this repo (it's just the upstream
project's static assets, easily re-fetched from the mjpg-streamer source if
`/mnt/UDISK/www/` is ever missing).

### 3. Copy the startup script

```bash
scp printer-init/start_streamer.sh root@10.168.113.245:/mnt/UDISK/
ssh root@10.168.113.245 "chmod +x /mnt/UDISK/start_streamer.sh"
```

### 4. Hook it into boot via rc.local

```bash
ssh root@10.168.113.245 "cat /etc/rc.local"
```

It should currently just say:
```
# Put your custom commands here that should be executed once
# the system init finished. By default this file does nothing.
exit 0
```

Replace it with:

```bash
ssh root@10.168.113.245 "cat > /etc/rc.local << 'EOF'
# Put your custom commands here that should be executed once
# the system init finished. By default this file does nothing.

/mnt/UDISK/start_streamer.sh &

exit 0
EOF"
```

### 5. Test with a real reboot

Power cycle the printer (physical switch or smart plug — don't rely on a
software reboot command, since we've never confirmed one exists/is safe).
After it's back up:

```bash
ssh root@10.168.113.245 "cat /mnt/UDISK/mjpg_streamer_boot.log"
ssh root@10.168.113.245 "ps | grep mjpg_streamer"
```

You should see log entries showing it waited for `/dev/video0`, then launched,
and a `ps` entry for the running `mjpg_streamer` process.

## Why not `/etc/rc.d/S81...` (OpenWrt-style init)?

This was the first approach tried, matching the pattern of `S80camera` and
friends (`START=81`, `USE_PROCD=1`, a `start_service()` function). It looked
identical in structure to the stock scripts, ran fine when invoked *manually*
(`/etc/rc.d/S81mjpg_streamer start`), but **never actually executed at real
boot time** — no log output at all, meaning the script's own logic never even
ran.

Best guess: `USE_PROCD=1` tells `rc.common` to hand off service management to
OpenWrt's procd daemon, which this stripped-down Tina Linux build likely
doesn't actually run — so the script silently hung waiting on a framework
that isn't there, rather than falling through to `start_service()`'s actual
loop.

Rather than debug procd's presence/absence further, we switched to
`/etc/rc.local` — a much simpler, framework-free "runs once at the end of
boot" hook that's guaranteed to execute since it's just a plain shell script.
This is the approach documented above and confirmed working across multiple
real reboots.

## Port choice: 8081, not 8080

Creality's `uvc_stream` binds port 8080. Even though we disable it (step 1
above), the port was deliberately chosen as 8081 so that re-enabling
`S80camera` in the future (e.g. to compare behavior, or if Creality ever
ships a firmware fix) wouldn't immediately conflict with our streamer.
