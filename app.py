#!/usr/bin/env python3
import subprocess, shlex, os
from flask import Flask, jsonify, request, send_from_directory

app = Flask(__name__, static_folder=None)

BASE_DIR = "/home/pi/creality-remote-control"
HOST = "root@10.168.113.245"
KEY = "/home/pi/.ssh/halot_key"
USB_PATH = "/mnt/exUDISK"
INTERNAL_PATH = "/mnt/UDISK"

def ssh_run(cmd):
    full = ["ssh", "-i", KEY, "-o", "ConnectTimeout=5", HOST, cmd]
    result = subprocess.run(full, capture_output=True, text=True)
    return result.stdout, result.stderr

@app.route("/")
def index():
    return send_from_directory(BASE_DIR, "index.html")

@app.route("/api/usb-files")
def usb_files():
    out, err = ssh_run(f"ls {USB_PATH}/*.cxdlp 2>/dev/null")
    files = [f.strip().rsplit("/", 1)[-1] for f in out.splitlines() if f.strip()]
    return jsonify(files)

@app.route("/api/load-file", methods=["POST"])
def load_file():
    filename = request.json.get("filename", "")
    if not filename or "/" in filename or ".." in filename:
        return jsonify({"error": "invalid filename"}), 400
    src = shlex.quote(f"{USB_PATH}/{filename}")
    dest = shlex.quote(f"{INTERNAL_PATH}/{filename}")

    # Remove any existing .cxdlp files in UDISK first (single-file enforcement)
    clear_out, clear_err = ssh_run(f"rm -f {INTERNAL_PATH}/*.cxdlp")

    out, err = ssh_run(f"cp {src} {dest}")
    if err:
        return jsonify({"error": err}), 500
    return jsonify({"status": "ok", "filename": filename})

@app.route("/api/udisk-files")
def udisk_files():
    out, err = ssh_run(f"ls {INTERNAL_PATH}/*.cxdlp 2>/dev/null")
    files = [f.strip().rsplit("/", 1)[-1] for f in out.splitlines() if f.strip()]
    return jsonify(files)

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080, threaded=True)
