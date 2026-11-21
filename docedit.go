package main

// Editing Markdown documents from the visual block editor.
// Who can edit: the owner, and admins for any Markdown document they can see.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// mdName turns a user title into a safe file name ending in .md (only a real .md/.markdown
// suffix is removed, so "Riunione 16.03" stays "Riunione 16.03.md").
func mdName(title string) string {
	n := strings.TrimSpace(title)
	low := strings.ToLower(n)
	for _, ext := range []string{".markdown", ".md"} {
		if strings.HasSuffix(low, ext) {
			n = n[:len(n)-len(ext)]
			break
		}
	}
	n = strings.TrimSpace(n)
	if n == "" {
		return ".md"
	}
	return cleanName(n) + ".md"
}

func isMarkdown(d Document) bool {
	ext := strings.ToLower(filepath.Ext(d.Name))
	return ext == ".md" || ext == ".markdown" || strings.Contains(d.Mime, "markdown")
}

// canEditDoc: owner, admins, or the leader of a group the document is shared with.
func (a *App) canEditDoc(u *User, d Document) bool {
	return isMarkdown(d) && (d.OwnerID == u.ID || u.IsAdmin() || a.leadsGroupOf(u, d.ID))
}

func (a *App) markEditable(u *User, docs []Document) {
	for i := range docs {
		docs[i].CanEdit = a.canEditDoc(u, docs[i])
	}
}

func (a *App) docVersion(id int64) int64 {
	var v int64
	a.db.QueryRow(`SELECT version FROM documents WHERE id=?`, id).Scan(&v)
	return v
}

// handleSaveDocContent overwrites a Markdown document. The client sends the version it
// started from; if someone saved in the meantime we refuse instead of losing their work.
func (a *App) handleSaveDocContent(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	d, err := a.accessibleDoc(u, pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !a.canEditDoc(u, d) {
		jsonErr(w, 403, "forbidden")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var in struct {
		Content string `json:"content"`
		Version int64  `json:"version"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if cur := a.docVersion(d.ID); in.Version != 0 && in.Version != cur {
		jsonErr(w, 409, "edited_elsewhere")
		return
	}
	name, err := a.writeDoc(u, d, in.Content, in.Name, false)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	a.liveAll("docs")
	jsonOK(w, map[string]any{"ok": true, "version": a.docVersion(d.ID), "name": name, "size": len(in.Content)})
}

// writeDoc replaces the content (and optionally the title) of a Markdown document.
// Permissions are checked by the caller (canEditDoc).
func (a *App) writeDoc(u *User, d Document, content, newName string, viaAI bool) (string, error) {
	size := int64(len(content))
	if size > 2<<20 {
		return "", fmt.Errorf("file_too_large")
	}
	if a.usedSpace(d.OwnerID)-d.Size+size > a.userQuota(d.OwnerID) {
		return "", fmt.Errorf("quota_exceeded")
	}
	a.keepVersion(d, content) // previous content goes into the history (last 10)
	full := filepath.Join(a.userDir(d.OwnerID), d.path)
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o640); err != nil {
		return "", fmt.Errorf("server_error")
	}
	if err := os.Rename(tmp, full); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("server_error")
	}
	name := d.Name
	if n := mdName(newName); n != ".md" {
		name = n
	}
	a.db.Exec(`UPDATE documents SET size=?, name=?, version=version+1, updated_at=?, updated_by=?, updated_via_ai=? WHERE id=?`, size, name, time.Now().Unix(), u.ID, viaAI, d.ID)
	return name, nil
}

// handleNewDoc creates an empty Markdown document to be filled in the editor.
func (a *App) handleNewDoc(w http.ResponseWriter, r *http.Request) {
	if !a.docCreateOn() {
		jsonErr(w, 403, "create_disabled")
		return
	}
	u := currentUser(r)
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var in struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	name := mdName(in.Name)
	if name == ".md" {
		name = mdName(fmt.Sprintf("Documento %s", time.Now().In(a.cfg.TZ).Format("02-01-2006 15h04")))
	}
	content := in.Content
	if strings.TrimSpace(content) == "" {
		content = "# " + strings.TrimSuffix(name, ".md") + "\n"
	}
	d, err := a.storeDoc(u.ID, name, "text/markdown", strings.NewReader(content), false)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	d.CanEdit = true
	jsonOK(w, d)
}
