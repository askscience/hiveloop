package main

// Group cover image: set by the group's leader or an admin. The whole photo is kept (the
// group page shows it in full), at most groupImageSide px on its longest side:
//   - WebP made by the browser (Chrome, Android, Firefox) is checked and kept as is when small;
//   - anything else (JPEG from Safari, PNG, oversized files) is resized and re-encoded on the
//     server as a compact JPEG (Go has no pure-Go WebP encoder).
// Files live under files/_groups so backups include them.

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	groupImageMax    = 2 << 20 // upload limit
	groupImageSide   = 1080    // longest side at most (a phone screen's width in pixels)
	groupImageKeep   = 200 << 10
	groupImageQ      = 62 // JPEG quality for server re-encodes
	groupImageMaxDim = 5000
)

func (a *App) groupImagePath(id int64) string {
	return filepath.Join(a.cfg.DataDir, "files", "_groups", strconv.FormatInt(id, 10)+".img")
}

// optimizeImage returns the bytes to store: small, valid WebP is kept; the rest becomes a
// JPEG whose longest side is at most groupImageSide.
func optimizeImage(b []byte) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 16 || cfg.Height < 16 || cfg.Width > groupImageMaxDim || cfg.Height > groupImageMaxDim {
		return nil, errBadInput
	}
	if format == "webp" && len(b) <= groupImageKeep && cfg.Width <= 1600 && cfg.Height <= 1600 {
		if _, _, err := image.Decode(bytes.NewReader(b)); err != nil {
			return nil, errBadInput
		}
		return b, nil
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, errBadInput
	}
	r := src.Bounds()
	w, h := r.Dx(), r.Dy()
	tw, th := w, h
	if m := max(w, h); m > groupImageSide {
		tw, th = w*groupImageSide/m, h*groupImageSide/m
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src) // transparent PNG: white background
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, r, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: groupImageQ}); err != nil {
		return nil, err
	}
	if format == "webp" && len(b) < out.Len() { // already lighter than our JPEG
		return b, nil
	}
	return out.Bytes(), nil
}

func writeFileAtomic(p string, b []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (a *App) handleSetGroupImage(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	b, err := io.ReadAll(io.LimitReader(r.Body, groupImageMax+1))
	if err != nil || len(b) == 0 {
		jsonErr(w, 400, "bad_request")
		return
	}
	if len(b) > groupImageMax {
		jsonErr(w, 413, "file_too_large")
		return
	}
	out, err := optimizeImage(b)
	if err != nil {
		jsonErr(w, 415, "bad_image")
		return
	}
	p := a.groupImagePath(id)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil || writeFileAtomic(p, out) != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	v := time.Now().UnixNano() / 1e6
	a.db.Exec(`UPDATE groups SET image_v=? WHERE id=?`, v, id)
	jsonOK(w, map[string]any{"ok": true, "image_v": v, "size": len(out)})
}

// optimizeGroupImages shrinks covers uploaded before the optimisation existed (runs once
// per file: optimised files are small and skipped next time).
func (a *App) optimizeGroupImages() {
	dir := filepath.Join(a.cfg.DataDir, "files", "_groups")
	files, _ := filepath.Glob(filepath.Join(dir, "*.img"))
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil || len(b) <= groupImageKeep/2 {
			continue
		}
		out, err := optimizeImage(b)
		if err != nil || len(out) >= len(b) {
			continue
		}
		if writeFileAtomic(p, out) == nil {
			id, _ := strconv.ParseInt(filepath.Base(p[:len(p)-4]), 10, 64)
			a.db.Exec(`UPDATE groups SET image_v=? WHERE id=?`, time.Now().UnixNano()/1e6, id)
			log.Printf("group image %d: %d KB -> %d KB", id, len(b)>>10, len(out)>>10)
		}
	}
}

func (a *App) handleDeleteGroupImage(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	os.Remove(a.groupImagePath(id))
	a.db.Exec(`UPDATE groups SET image_v=0 WHERE id=?`, id)
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleGroupImage(w http.ResponseWriter, r *http.Request) {
	if !a.canSeeGroup(currentUser(r), pathID(r, "id")) {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(a.groupImagePath(pathID(r, "id")))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := "image/jpeg"
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG")):
		ct = "image/png"
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		ct = "image/webp"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// URLs carry ?v=<version>, so the browser may keep the image
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Write(b)
}
