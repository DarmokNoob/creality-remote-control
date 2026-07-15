# Camera Streamer Notes

Scoped notes specific to the `mjpg_streamer` camera feed setup — why it's
built this way, not general deployment steps (see the main
[`README.md`](../README.md) for that).

## Why Creality's stock camera service is disabled

Creality's own `uvc_stream` binary (launched by `S80camera`) is unstable —
it segfaults reliably within about a minute under any real client load (see
[`docs/touch-injection-notes.md`](../docs/touch-injection-notes.md) for the
unrelated investigation that led to fully understanding this binary via
disassembly and `strace`). It also holds `/dev/video0` exclusively, so it
must be disabled for our own streamer to get the camera at all.

```bash
mv /etc/rc.d/S80camera /etc/rc.d/.S80camera
```

Renaming (not deleting) means it's easy to restore if needed, and the
leading `.` makes the init system's `S*`/`K*` glob pattern skip it.

## Why not `/etc/rc.d/S81...` (OpenWrt-style init)?

The first approach tried matched the pattern of `S80camera` and friends
(`START=81`, `USE_PROCD=1`, a `start_service()` function). It looked
identical in structure to the stock scripts, ran fine when invoked
*manually* (`/etc/rc.d/S81mjpg_streamer start`), but **never actually
executed at real boot time** — no log output at all, meaning the script's
own logic never even ran.

Best guess: `USE_PROCD=1` tells `rc.common` to hand off service management
to OpenWrt's procd daemon, which this stripped-down Tina Linux build likely
doesn't actually run — so the script silently hung waiting on a framework
that isn't there, rather than falling through to `start_service()`'s actual
loop.

Rather than debug procd's presence/absence further, the setup switched to
`/etc/rc.local` — a much simpler, framework-free "runs once at the end of
boot" hook that's guaranteed to execute since it's just a plain shell
script. This is the approach `start.sh` implements, confirmed working
across multiple real reboots.

## Port choice: 8081, not 8080

Creality's `uvc_stream` binds port 8080. Even though it's disabled, the
port for our own streamer was deliberately chosen as 8081 so that
re-enabling `S80camera` in the future (e.g. to compare behavior, or if
Creality ever ships a firmware fix) wouldn't immediately conflict.

## BusyBox gotchas specific to running mjpg_streamer here

This device's BusyBox build is missing several tools that most guides
assume exist:

- No `pkill` — use `ps | grep <name> | grep -v grep | awk '{print $1}' | xargs -r kill`
- No `nohup` — plain `&` backgrounding is sufficient here; don't bother
  trying to install or work around this, it just wastes time
- No `file` — use `hexdump -C <path> | head` or `od -A x -t x1z <path> | head`
  to inspect binaries by hand if architecture/format is ever in question
