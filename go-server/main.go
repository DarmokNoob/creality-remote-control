package main

import (
        "encoding/json"
        "fmt"
        "image"
        "image/color"
        "image/png"
        "io"
        "log"
        "net/http"
        "os"
        "path/filepath"
        "strings"
)

const (
        thumbOffset = 27009  // Thumbnail1 (290x290)
        thumbWidth  = 290
        thumbHeight = 290
        usbPath      = "/mnt/exUDISK"
        internalPath = "/mnt/UDISK"
        staticDir    = "/mnt/UDISK/halot-remote-control/www-ui"
        listenAddr   = ":8082"
)

func listCxdlpFiles(dir string) ([]string, error) {
        entries, err := os.ReadDir(dir)
        if err != nil {
                return nil, err
        }
        var files []string
        for _, e := range entries {
                if e.IsDir() {
                        continue
                }
                if strings.HasSuffix(strings.ToLower(e.Name()), ".cxdlp") {
                        files = append(files, e.Name())
                }
        }
        if files == nil {
                files = []string{}
        }
        return files, nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func handleUsbFiles(w http.ResponseWriter, r *http.Request) {
        files, err := listCxdlpFiles(usbPath)
        if err != nil {
                writeJSONError(w, 500, err.Error())
                return
        }
        writeJSON(w, files)
}

func handleUdiskFiles(w http.ResponseWriter, r *http.Request) {
        files, err := listCxdlpFiles(internalPath)
        if err != nil {
                writeJSONError(w, 500, err.Error())
                return
        }
        writeJSON(w, files)
}

type loadFileRequest struct {
        Filename string `json:"filename"`
}

func handleLoadFile(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
                writeJSONError(w, 405, "method not allowed")
                return
        }

        var req loadFileRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                writeJSONError(w, 400, "invalid request body")
                return
        }

        if req.Filename == "" || strings.Contains(req.Filename, "/") || strings.Contains(req.Filename, "..") {
                writeJSONError(w, 400, "invalid filename")
                return
        }

        srcPath := filepath.Join(usbPath, req.Filename)
        dstPath := filepath.Join(internalPath, req.Filename)

        if _, err := os.Stat(srcPath); err != nil {
                writeJSONError(w, 404, "source file not found on USB drive")
                return
        }

        existing, err := listCxdlpFiles(internalPath)
        if err != nil {
                writeJSONError(w, 500, "failed to list existing files: "+err.Error())
                return
        }
        for _, name := range existing {
                if err := os.Remove(filepath.Join(internalPath, name)); err != nil {
                        log.Printf("warning: failed to remove %s: %v", name, err)
                }
        }

        if err := copyFile(srcPath, dstPath); err != nil {
                writeJSONError(w, 500, "copy failed: "+err.Error())
                return
        }

        writeJSON(w, map[string]string{"status": "ok", "filename": req.Filename})
}

func copyFile(src, dst string) error {
        in, err := os.Open(src)
        if err != nil {
                return err
        }
        defer in.Close()

        out, err := os.Create(dst)
        if err != nil {
                return err
        }
        defer out.Close()

        _, err = io.Copy(out, in)
        if err != nil {
                return err
        }
        return out.Sync()
}

func decodeThumbnail(path string) (image.Image, error) {
        f, err := os.Open(path)
        if err != nil {
                return nil, err
        }
        defer f.Close()

        rawSize := thumbWidth * thumbHeight * 2
        raw := make([]byte, rawSize)

        _, err = f.Seek(int64(thumbOffset), 0)
        if err != nil {
                return nil, err
        }
        _, err = io.ReadFull(f, raw)
        if err != nil {
                return nil, err
        }

        // Tint color target (light blue, #4697bd)
        const tintR, tintG, tintB = 0x46, 0x97, 0xbd

        // First pass: decode RGB565 and extract blue channel intensities
        blueVals := make([]uint8, thumbWidth*thumbHeight)
        idx := 0
        minBlue, maxBlue := uint8(255), uint8(0)
        for i := 0; i < thumbWidth*thumbHeight; i++ {
                hi := raw[idx]
                lo := raw[idx+1]
                val := uint16(hi)<<8 | uint16(lo)
                b := uint8(val&0x1F) << 3
                blueVals[i] = b
                if b < minBlue {
                        minBlue = b
                }
                if b > maxBlue {
                        maxBlue = b
                }
                idx += 2
        }

        // Avoid divide-by-zero if the image is a flat color
        rangeVal := float64(maxBlue) - float64(minBlue)
        if rangeVal < 1 {
                rangeVal = 1
        }

        img := image.NewRGBA(image.Rect(0, 0, thumbWidth, thumbHeight))
        i := 0
        for y := 0; y < thumbHeight; y++ {
                for x := 0; x < thumbWidth; x++ {
                        // Normalize blue value to 0.0-1.0 range
                        norm := (float64(blueVals[i]) - float64(minBlue)) / rangeVal
                        if norm < 0 {
                                norm = 0
                        }
                        if norm > 1 {
                                norm = 1
                        }

                        // Map normalized intensity onto tint color (black -> tint)
                        r := uint8(norm * tintR)
                        g := uint8(norm * tintG)
                        bl := uint8(norm * tintB)

                        img.Set(x, y, color.RGBA{r, g, bl, 255})
                        i++
                }
        }

        return img, nil
}

func handleThumbnail(w http.ResponseWriter, r *http.Request) {
        filename := r.URL.Query().Get("filename")
        if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "..") {
                writeJSONError(w, 400, "invalid filename")
                return
        }

        fullPath := filepath.Join(internalPath, filename)
        img, err := decodeThumbnail(fullPath)
        if err != nil {
                writeJSONError(w, 500, "failed to decode thumbnail: "+err.Error())
                return
        }

        w.Header().Set("Content-Type", "image/png")
        png.Encode(w, img)
}

func main() {
        mux := http.NewServeMux()

        mux.HandleFunc("/api/usb-files", handleUsbFiles)
        mux.HandleFunc("/api/udisk-files", handleUdiskFiles)
        mux.HandleFunc("/api/load-file", handleLoadFile)
        mux.HandleFunc("/api/thumbnail", handleThumbnail)

        fs := http.FileServer(http.Dir(staticDir))
        mux.Handle("/", fs)

        fmt.Printf("halot-server listening on %s, serving static from %s\n", listenAddr, staticDir)
        log.Fatal(http.ListenAndServe(listenAddr, mux))
}