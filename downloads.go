package main

// App downloads, public (shown on the sign-up page):
//   /download/hiveloop.apk          Android app built by GitHub Actions (see android/README.md: put it in data/downloads)
//   /download/hiveloop.mobileconfig iOS "Web Clip" profile: Hiveloop icon on the home screen, opens full screen

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) apkPath() string { return filepath.Join(a.cfg.DataDir, "downloads", "hiveloop.apk") }

// publicURL is the address users reach Hiveloop at (HIVELOOP_PUBLIC_URL, or the request host).
func (a *App) publicURL(r *http.Request) string {
	if u := env("HIVELOOP_PUBLIC_URL", ""); u != "" {
		return strings.TrimRight(u, "/") + "/"
	}
	if r == nil { // no request (e.g. the assistant): relative links
		return "/"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return "https://" + host + "/"
}

func (a *App) handleDownloadsInfo(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"android": false}
	if st, err := os.Stat(a.apkPath()); err == nil {
		out["android"] = true
		out["android_size"] = st.Size()
		out["android_date"] = st.ModTime().Unix()
	}
	jsonOK(w, out)
}

func (a *App) handleAPK(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(a.apkPath())
	if err != nil {
		http.Error(w, "Android app not available yet", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="Hiveloop.apk"`)
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func uuidFrom(s string) string {
	h := sha1.Sum([]byte(s))
	h[6] = (h[6] & 0x0f) | 0x50
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func xmlEsc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func (a *App) handleMobileConfig(w http.ResponseWriter, r *http.Request) {
	url := a.publicURL(r)
	sub, _ := fs.Sub(webFS, "web")
	icon, _ := fs.ReadFile(sub, "apple-touch-icon.png")
	clip, prof := uuidFrom("hiveloop-webclip:"+url), uuidFrom("hiveloop-profile:"+url)
	body := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>FullScreen</key><true/>
			<key>Icon</key><data>` + base64.StdEncoding.EncodeToString(icon) + `</data>
			<key>IsRemovable</key><true/>
			<key>Label</key><string>Hiveloop</string>
			<key>Precomposed</key><true/>
			<key>URL</key><string>` + xmlEsc(url) + `</string>
			<key>PayloadDisplayName</key><string>Hiveloop</string>
			<key>PayloadDescription</key><string>Aggiunge Hiveloop alla schermata Home / Adds Hiveloop to the Home Screen</string>
			<key>PayloadIdentifier</key><string>app.hiveloop.webclip.` + clip + `</string>
			<key>PayloadType</key><string>com.apple.webClip.managed</string>
			<key>PayloadUUID</key><string>` + clip + `</string>
			<key>PayloadVersion</key><integer>1</integer>
		</dict>
	</array>
	<key>PayloadDisplayName</key><string>Hiveloop</string>
	<key>PayloadDescription</key><string>Icona di Hiveloop sulla schermata Home, si apre a schermo intero. Nessun altro permesso. / Hiveloop icon on the Home Screen, opens full screen. No other permissions.</string>
	<key>PayloadOrganization</key><string>Hiveloop</string>
	<key>PayloadIdentifier</key><string>app.hiveloop.profile.` + prof + `</string>
	<key>PayloadRemovalDisallowed</key><false/>
	<key>PayloadType</key><string>Configuration</string>
	<key>PayloadUUID</key><string>` + prof + `</string>
	<key>PayloadVersion</key><integer>1</integer>
</dict>
</plist>
`
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="Hiveloop.mobileconfig"`)
	w.Write([]byte(body))
}
