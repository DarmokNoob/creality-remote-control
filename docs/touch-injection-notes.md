# Remote "Return to Idle" — Investigation Notes

## The problem

After a print finishes (or is stopped), `PrinterUI` reports `printStatus: PRINT_END`
via the WebSocket API and stays there. A new `START_PRINT` command sent while in
this state is silently ignored — the printer only accepts a new print once it's
back in `PRINT_GENERAL` (idle) state.

The **only known way** to clear `PRINT_END` and return to `PRINT_GENERAL` is
physically tapping the Back arrow on the touchscreen, or power-cycling the unit.
This note documents why that's the case and what was tried to work around it
remotely, across two sessions.

## What's confirmed

- There is no WebSocket command that clears `PRINT_END`. `PRINT_IDLE` and
  `PRINT_COMPLETE` were tried directly and both were silently ignored (no
  status change in the following `GET_PRINT_STATUS` responses).
- Disassembly of `/usr/bin/PrinterUI/PrinterUI` (via radare2) shows the
  Back button's handler — `PrintFile::returnButtonClicked()` at `0x00148e38` —
  does exactly one thing: calls `QMetaObject::activate()`, Qt's internal
  signal-emission mechanism. It's pure local UI navigation. No network
  activity, no command construction, nothing that goes over the WebSocket.
  **There is no remote equivalent to find, because the button doesn't use one.**

## Session 1: Touch-injection attempts (both failed)

Given the above, the next idea was simulating the physical tap itself at the
Linux input-device level via SSH, rather than trying to find a nonexistent
software command.

**Touch panel identified:** `/dev/input/event2`, name `cxsw_ctp`
("Creality Software Capacitive Touch Panel"), Multi-Touch Protocol Type B.
Reports `ABS_MT_POSITION_X`/`Y` in range 0–800 / 0–480.

### Attempt 1: Write directly to `/dev/input/event2`

A small C program (`tap.c`) opened the real device node and wrote
`ABS_MT_POSITION_X/Y`, `BTN_TOUCH`, and `SYN_REPORT` events directly,
mimicking a real touch sequence.

- All `write()` calls succeeded with no errors.
- No visible effect on screen, even when targeting large, unambiguous
  buttons (e.g. the main menu's "Print" button).
- **Why it failed:** writing synthetic `ABS`/`KEY` events directly to a real
  hardware device's `/dev/input/eventN` node is not how Linux input injection
  works. The kernel's evdev layer accepts the write syscall but doesn't feed
  those events into the input subsystem for a physical device — the OS
  silently discards them. This is a fundamental limitation of the technique,
  not specific to this printer.

### Attempt 2: Virtual device via `/dev/uinput`

A second program (`utap.c`) created a proper virtual input device through
`/dev/uinput` (the correct, standard mechanism the kernel provides for
synthetic input) and sent the same tap sequence through it.

- Device creation succeeded (`/dev/uinput` exists and is writable as root).
- Still no visible effect, even on large buttons.
- **Why it failed:** checking `PrinterUI`'s running environment via
  `/proc/<pid>/environ` revealed:
  ```
  QT_QPA_EVDEV_TOUCHSCREEN_PARAMETERS=evdev:/dev/input/event2:rotate=0
  ```
  Qt is hardcoded to read touch input from `/dev/input/event2` specifically,
  by exact device path — not scanning for touch-capable devices generically.
  A `uinput`-created virtual device gets an arbitrary, different `eventN`
  node, so Qt never looks at it.

## Session 2: Deeper investigation

Picked back up with two open questions: (1) does Qt actually discover new
input devices dynamically at all (even if it doesn't treat them as touch
sources), and (2) can the environment variable itself just be pointed at our
virtual device instead of fighting to impersonate `event2`?

### Full boot environment recovered

Found `/etc/rc.d/S99cxpm-ui`, the real init script that launches `PrinterUI`
at boot. It sets several environment variables our earlier manual relaunches
were missing:

```sh
export QT_QPA_EGLFS_ROTATION=0
export QT_QPA_EGLFS_INTEGRATION=none
export XDG_RUNTIME_DIR=/dev/shm
export QT_QPA_EVDEV_TOUCHSCREEN_PARAMETERS='evdev:/dev/input/event2:rotate=0'
export QT_QPA_GENGERIC_PLUGINS='evdevtouch:/dev/input/event2'
export HORSCREEN=1   # only for non-"vertical screen" products, which includes CL60R
```

`HORSCREEN=1` turned out to be the actual fix for the screen-rotation glitch
seen during manual relaunches in Session 1 — without it, the UI renders
sideways because the physical panel is electrically portrait (`540x2560`,
set by `/etc/init.d/hdmi_init`) and needs this flag to render landscape in
software. `$product` also needs to be exported manually (`CL60R` for this
unit) since it's normally set earlier in the real boot sequence and isn't
present in an interactive SSH shell — without it, `hdmi_init` takes the
wrong code path entirely and throws GPIO errors.

With the full environment (including `HORSCREEN=1` and `product=CL60R`)
and `hdmi_init` re-run first, a manual relaunch renders correctly in
landscape — confirming the rotation issue was fully understood and fixable,
just unrelated to the actual touch-injection problem.

### Confirmed: touch coordinates follow rotation correctly

Wrote a raw event reader (`readtouch.c`) and had the user physically tap
known buttons while it was listening. Confirmed real, working coordinates
(e.g. Settings at `(544, 291)`, Back at `(0, 470)` in one screen state).
Physically tapping the *visually rendered* location of a button — even
mid-rotation-glitch — worked correctly, proving the touch driver's
coordinate mapping was never the problem.

### uinput device confirmed visible to Qt, but not used for touch

Created a **persistent** virtual uinput device (`touchsrv.c`, unlike the
earlier one-shot `utap.c`) so it could be tested interactively. Confirmed
via `strace -f -p <PrinterUI pid> -e trace=open,openat` that the moment the
virtual device appeared, `PrinterUI` opened `/sys/devices/virtual/input/input4/uevent`
— i.e. Qt's udev-based device monitoring *does* notice new input devices
generically.

However:

- Sending taps to this virtual device (at its real assigned path, `event4`)
  produced no UI reaction, even using coordinates already confirmed correct
  for the real device.
- Relaunching `PrinterUI` with `QT_QPA_EVDEV_TOUCHSCREEN_PARAMETERS` and
  `QT_QPA_GENGERIC_PLUGINS` pointed **directly at `event4`** instead of
  `event2` (sacrificing real touch for the test) still produced no reaction
  to taps sent via `touchsrv`.
- A targeted `strace -f -y -p <pid> -e trace=read,ioctl` (with `-y` to
  annotate file descriptors with their real paths) attached *after* Qt's
  initial startup showed no `read`/`ioctl` activity on `event2` **or**
  `event4` during a tap — but this is inconclusive on its own, since no
  physical control-case tap was performed during the same capture window
  to confirm the trace would have caught real touch activity if it occurred.

**Net result:** the discovery/uevent mechanism is real, but nothing tested
this session got a synthetic tap to actually register as input, even when
explicitly pointing Qt's own touch config at the synthetic device. The most
likely explanation, based on the evidence gathered, is that Qt's udev
awareness is separate from its actual touch-input backend, and the specific
`evdevtouch` handler may perform capability checks or expect properties a
`uinput`-created device doesn't fully replicate (was not narrowed down
further).

## Why this wasn't pursued further

The remaining untested paths — forcing a virtual device to claim the literal
`event2` path, or unbinding/rebinding the real `cxsw_ctp` driver — both risk
interfering with the real, physical touchscreen on a printer in daily use.
Given two full sessions of testing without a working result, and the
increasing complexity/risk of the remaining options, this was set aside
rather than pushed further tonight.

## Current status / workaround

**Accepted limitation:** a physical tap on the Back button, or a full power
cycle, is required between a completed/stopped print and starting a new one
remotely. This is a minor manual step, not a major workflow blocker — the
rest of the remote pipeline (USB file browsing, load-to-queue, start/pause/
stop, live camera feed) works fully remotely and persists across reboots.

## Possible future directions (untested)

- Investigate what specific properties/capabilities Qt's `evdevtouch` plugin
  checks for on a device before treating it as a touch source (would require
  either Qt source inspection or further disassembly of the plugin itself,
  not just `PrinterUI`).
- Investigate whether `PrinterUI` can be relaunched with a modified
  `QT_QPA_EVDEV_TOUCHSCREEN_PARAMETERS` pointing at *both* `event2` and a
  synthetic device (comma-separated, if the plugin supports multiple
  devices) — untested this session due to time, but lower-risk than
  device-path spoofing since it doesn't require touching the real driver.
- Look for a udev rule or kernel-level way to make a `uinput` virtual device
  present itself as `/dev/input/event2` (e.g. by unbinding the real driver
  first, then rebinding after). Higher risk — same caveat as before.
- A physical/mechanical actuator (small servo or solenoid) that taps the
  actual screen location after detecting `PRINT_END` via the WebSocket API.
  Doesn't touch the printer's software/firmware at all, so no risk of
  breaking real touch input — just a different kind of project.
