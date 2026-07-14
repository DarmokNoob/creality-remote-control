#!/usr/bin/env python3
import sys, os, json, base64, zlib, struct
import websocket
from Crypto.Cipher import DES
from Crypto.Util.Padding import pad

PRINTER_WS = "ws://10.168.113.245:18188/"
PASSWORD = "0"
DES_KEY = bytes.fromhex("6138356539643638")
CHUNK_SIZE = 0x10000

def make_token(password):
    cipher = DES.new(DES_KEY, DES.MODE_ECB)
    padded = pad(password.encode(), DES.block_size)
    encrypted = cipher.encrypt(padded)
    return base64.b64encode(encrypted).decode()

def push_file(filepath):
    filename = os.path.basename(filepath)
    with open(filepath, 'rb') as f:
        data = f.read()
    size = len(data)
    token = make_token(PASSWORD)

    print(f"Connecting to {PRINTER_WS} ...")
    ws = websocket.create_connection(PRINTER_WS)

    def send_json(cmd, extras=None):
        msg = {"cmd": cmd, "token": token}
        if extras:
            msg.update(extras)
        ws.send(json.dumps(msg))

    def send_chunk(offset):
        chunk = data[offset:offset+CHUNK_SIZE]
        if not chunk:
            return
        compressed = zlib.compress(chunk)
        length_prefix = struct.pack(">I", len(chunk))
        ws.send_binary(length_prefix + compressed)

    print(f"Starting upload: {filename} ({size} bytes)")
    send_json("START_FILE", {
        "filename": filename,
        "offset": "0",
        "size": str(size),
    })

    last_percent = -1
    while True:
        raw = ws.recv()
        if isinstance(raw, bytes):
            continue
        msg = json.loads(raw)
        cmd = msg.get("cmd")
        if cmd == "START_FILE":
            offset = int(msg.get("offset", 0))
            send_chunk(offset)
        elif cmd == "START_DATA":
            received = int(msg.get("received", 0))
            percent = int(received * 100 / size)
            if percent != last_percent:
                print(f"\rProgress: {percent}% ({received}/{size} bytes)", end="", flush=True)
                last_percent = percent
            if received >= size:
                print("\nUpload complete.")
                break
            send_chunk(received)
        elif cmd == "CHECK_DATA":
            print(f"\nCheck: {msg}")
        else:
            print(f"\nUnexpected message: {msg}")

    ws.close()
    print("File is now on the printer. Use the touchscreen or web UI to start the print.")

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python3 push_to_printer.py <path-to-file.cxdlp>")
        sys.exit(1)
    push_file(sys.argv[1])
