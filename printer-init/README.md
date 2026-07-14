# Printer Init Scripts

These scripts belong on the Halot One itself, not the Pi. They are version-controlled here for reference and easy redeployment after a firmware update wipes /overlay.

## Deployment

```bash
scp -i ~/.ssh/halot_key printer-init/S81mjpg_streamer root@10.168.113.245:/etc/rc.d/
ssh -i ~/.ssh/halot_key root@10.168.113.245 "chmod +x /etc/rc.d/S81mjpg_streamer"
```

Runs on port 8081 (not 8080) so it can coexist with Creality's stock uvc_stream without a port conflict.

Also requires `mjpg_streamer`, `input_uvc.so`, and `output_http.so` (from `bin/` in this repo) to be present at `/mnt/UDISK/` on the printer.

## Why S81

S80camera (Creality's own crash-prone streamer) runs at priority 80. This runs one step after, at 81, so it doesn't race with the stock service on boot. Both currently run independently — Creality's `uvc_stream` binds port 8080 too, so **only one can run at a time**. If S80camera's stock behavior causes a conflict, consider disabling it (rename to `.S80camera`) to avoid a port clash.
