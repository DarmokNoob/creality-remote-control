# Rooting a Creality TinaLinux Printer over ADB (USB)

This documents the process used to gain root SSH access to a Creality Halot
One (model `CL60R`) over USB, without any prior authentication. The
underlying technique — pulling and patching `/etc/shadow` via ADB's file
sync protocol — is not specific to this printer model; it relies on
behavior of the TinaLinux/Allwinner ADB daemon shipped across several
Creality printers. Model-specific details (paths, init script names) are
called out separately so they can be re-verified on other units rather than
assumed.

## Why this works at all

Creality's printers ship with `adbd` (Android Debug Bridge daemon) running
and reachable over USB, with **no authentication required for file
transfer**. ADB's `push`/`pull` operations use a separate protocol service
(`sync:`) from interactive shell access (`shell:`), and on these devices
`sync:` requires no login at all — you can read and write arbitrary files
as root before ever authenticating.

This means `/etc/shadow` (the file holding password hashes) can be pulled,
edited locally, and pushed back — effectively setting the root password to
anything you choose, without knowing the original.

## Prerequisites

- A host machine with USB access to the printer (a PC, or a Raspberry Pi
  wired directly to the printer's USB port)
- `adb` (Android Debug Bridge) — see architecture note below
- Python 3 with the `adb_shell` and `pycryptodome` packages (or just
  `adb_shell` — see note on `openssl` alternative)
- `openssl` (for computing password hashes) — usually preinstalled on Linux

## Step 1: Get a working `adb` binary

Standard package-manager `adb` builds are usually compiled for modern ARM
(ARMv7+) or x86_64. **If your host machine's CPU doesn't match**, the
native binary will crash immediately with an illegal instruction error.

**Specific case encountered:** a Raspberry Pi Zero W (ARMv6, single-core)
could not run the `android-tools-adb` package from Debian's repos at all —
it's built for ARMv7+. The fix was **QEMU user-mode emulation**:

```bash
sudo apt install qemu-user-static
qemu-arm-static $(which adb) devices
```

This transparently emulates the newer ARM instructions the binary needs.
It's slower than native but fully functional. **If your host is a normal
x86_64 PC or a newer Pi (3/4/5), you likely don't need this step at all** —
try native `adb` first.

## Step 2: Confirm the device is visible

```bash
adb devices
```

(Prefix with `qemu-arm-static` if using the emulation workaround above.)
You should see the printer listed by its serial number, status `device`
(not `offline` — if it shows `offline`, the ADB key hasn't been generated/
accepted yet; unplugging and replugging the USB cable after the first
connection attempt usually resolves this).

## Step 3: Pull `/etc/shadow`

No root or login needed for this — it's a plain file transfer.

Using the `adb_shell` Python library (works even when the native `adb`
binary's interactive shell doesn't — see note at the end):

```python
from adb_shell.adb_device import AdbDeviceUsb

device = AdbDeviceUsb()
device.connect(auth_timeout_s=5)
device.pull('/etc/shadow', 'shadow_backup')
print(open('shadow_backup').read())
```

Find the `root:` line. It looks like:

```
root:$1$SALTVALUE$HASHVALUE:0:0:99999:7:::
```

The `$1$` indicates MD5-crypt. Note the salt (the characters between the
first and second `$`).

## Step 4: Compute a new password hash

```bash
openssl passwd -1 -salt SALTVALUE "your-new-password"
```

Use the **same salt** found in step 3, so the line format stays consistent.
This outputs a new hash like `$1$SALTVALUE$newhashvalue`.

## Step 5: Edit and push back

Edit `shadow_backup` locally, replacing only the hash portion of the root
line with the new one from step 4. **Leave every other field unchanged**,
including the password-age field (the number right after the hash) — do
not zero it out. Setting that field to `0` can cause the system to treat
the password as immediately expired and force a change on next login,
which may fail outright on minimal BusyBox systems that lack the full
`passwd` toolchain needed to handle that flow gracefully.

```python
from adb_shell.adb_device import AdbDeviceUsb

device = AdbDeviceUsb()
device.connect(auth_timeout_s=5)
device.push('shadow_backup', '/etc/shadow')
```

## Step 6: Get an actual interactive root shell

This is the part that trips people up. `adb_shell`'s one-shot `.shell(cmd)`
method does **not** work for this — these devices wrap every one-shot shell
command through `/bin/login -c "<command>"`, and the minimal BusyBox
`login` binary on these systems doesn't support the `-c` flag at all,
causing every one-shot command to fail with a usage error, regardless of
whether the password is correct.

**You need a true interactive session instead**, which requires the native
`adb` binary (wrapped in QEMU if necessary, per Step 1):

```bash
qemu-arm-static $(which adb) shell
```

This drops you into a real `login:` prompt. Log in with `root` and the
password you set in Step 4. This works because a no-argument interactive
shell request invokes `/bin/login` directly (which supports normal
interactive prompts), rather than the broken `-c` wrapper path.

## Step 7: Set the password properly via `passwd`

Even though you already set a working password via the shadow edit, it's
worth running `passwd` once you have a real shell, rather than relying on
the manually-edited hash long-term:

```bash
passwd
```

This lets the system's own tooling regenerate the hash and correctly set
the password-age timestamp field, avoiding any edge cases from the manual
edit in Step 5.

## Step 8: Enable and start SSH

Check if `sshd` is already running:

```bash
ps | grep sshd
netstat -tlnp | grep :22
```

If it's running but connections are refused/rejected, check the config:

```bash
grep -iE "PermitRootLogin|PasswordAuthentication" /etc/ssh/sshd_config
```

Modern OpenSSH defaults to `PermitRootLogin prohibit-password` even when
not explicitly set, which blocks password-based root login even with a
correctly-configured account. Enable both explicitly:

```bash
sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config
sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config
```

Restart it. **The init script name/path varies by model/firmware** — on
the tested unit it was:

```bash
/etc/rc.d/S50sshd restart
```

If that path doesn't exist on your unit, search for it:

```bash
find /etc/rc.d /etc/init.d -iname "*sshd*"
```

## Step 9: Verify

From another machine on the same network as the printer:

```bash
ssh root@<printer-ip>
```

If this works, you now have persistent root SSH access over the network —
no further USB/ADB steps needed for future sessions.

## Known gotchas

- **OTA firmware updates likely reset the root password** back to
  whatever the stock firmware ships with. Re-run steps 3-7 after any
  firmware update if SSH access stops working.
- **A device showing `offline` in `adb devices`** usually just needs a
  cable unplug/replug after the first connection attempt generates a
  local ADB key.
- **Editing `/etc/shadow` by hand and zeroing the password-age field**
  causes a forced password-change flow on login that can fail outright on
  minimal BusyBox systems — always use the real salt and leave that field
  alone, or just fix it via `passwd` once you have shell access (Step 7).

## What's specific to this printer model (CL60R) vs. general

**General (worked without model-specific assumptions):**
- ADB `sync:` requiring no auth
- Shadow file format and patching approach
- The `/bin/login -c` limitation on one-shot shell commands
- `PermitRootLogin`/`PasswordAuthentication` needing explicit enabling

**Model/unit-specific (verify on other hardware before assuming):**
- `sshd` init script at `/etc/rc.d/S50sshd`
- Default/fallback root passwords (not tested on this unit, since we
  patched from scratch — the CreationFactory blog post referenced during
  research mentions `66668888` works on some newer firmware, but this
  wasn't independently confirmed here)
