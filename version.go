package main

// Interface version = hash of the embedded web files. Open pages poll it and reload
// themselves after a deploy (the Android app and installed PWAs keep pages open for days).

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sort"
	"sync"
)

var (
	uiVersionOnce sync.Once
	uiVersionVal  string
)

func uiVersion() string {
	uiVersionOnce.Do(func() {
		sub, _ := fs.Sub(webFS, "web")
		var paths []string
		fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				paths = append(paths, p)
			}
			return nil
		})
		sort.Strings(paths)
		h := sha256.New()
		for _, p := range paths {
			b, _ := fs.ReadFile(sub, p)
			h.Write([]byte(p))
			h.Write(b)
		}
		uiVersionVal = hex.EncodeToString(h.Sum(nil))[:12]
	})
	return uiVersionVal
}

func (a *App) handleVersion(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"version": uiVersion()})
}
