package main

// Version history for Markdown documents: before a .md file is overwritten (editor,
// assistant or restore) its previous content is kept. Only the last maxDocVersions are
// stored. Old versions are reachable only by people who can edit the document.

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const maxDocVersions = 10

const docVersionsSchema = `CREATE TABLE IF NOT EXISTS doc_versions (
  doc_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  name TEXT NOT NULL,
  content TEXT NOT NULL,
  saved_at INTEGER NOT NULL,
  saved_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  via_ai INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (doc_id, version)
)`

type DocVersion struct {
	Version int64  `json:"version"`
	Name    string `json:"name"`
	SavedAt int64  `json:"saved_at"`
	Author  string `json:"author"`
	ViaAI   bool   `json:"via_ai"`
	Size    int    `json:"size"`
	Text    string `json:"text,omitempty"`
}

// keepVersion stores the document's current content as a version, if it differs from
// the content about to be written.
func (a *App) keepVersion(d Document, newContent string) {
	old, err := os.ReadFile(filepath.Join(a.userDir(d.OwnerID), d.path))
	if err != nil || string(old) == newContent {
		return
	}
	var ver, created, updated int64
	var by sql.NullInt64
	var name string
	var viaAI, createdAI bool
	if err := a.db.QueryRow(`SELECT version,name,created_at,updated_at,updated_by,updated_via_ai,via_ai FROM documents WHERE id=?`, d.ID).
		Scan(&ver, &name, &created, &updated, &by, &viaAI, &createdAI); err != nil {
		return
	}
	savedAt, savedBy := updated, by
	if updated == 0 { // never edited: the original upload/creation
		savedAt, savedBy, viaAI = created, sql.NullInt64{Int64: d.OwnerID, Valid: true}, createdAI
	}
	a.db.Exec(`INSERT OR REPLACE INTO doc_versions(doc_id,version,name,content,saved_at,saved_by,via_ai) VALUES(?,?,?,?,?,?,?)`,
		d.ID, ver, name, string(old), savedAt, savedBy, viaAI)
	a.db.Exec(`DELETE FROM doc_versions WHERE doc_id=? AND version NOT IN
		(SELECT version FROM doc_versions WHERE doc_id=? ORDER BY version DESC LIMIT ?)`, d.ID, d.ID, maxDocVersions)
}

func (a *App) docVersions(docID int64) ([]DocVersion, error) {
	rows, err := a.db.Query(`SELECT v.version,v.name,v.saved_at,COALESCE(NULLIF(u.name,''),u.username,''),v.via_ai,length(CAST(v.content AS BLOB))
		FROM doc_versions v LEFT JOIN users u ON u.id=v.saved_by WHERE v.doc_id=? ORDER BY v.version DESC`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DocVersion{}
	for rows.Next() {
		var v DocVersion
		if err := rows.Scan(&v.Version, &v.Name, &v.SavedAt, &v.Author, &v.ViaAI, &v.Size); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (a *App) docVersionText(docID, ver int64) (DocVersion, error) {
	var v DocVersion
	err := a.db.QueryRow(`SELECT v.version,v.name,v.saved_at,COALESCE(NULLIF(u.name,''),u.username,''),v.via_ai,v.content
		FROM doc_versions v LEFT JOIN users u ON u.id=v.saved_by WHERE v.doc_id=? AND v.version=?`, docID, ver).
		Scan(&v.Version, &v.Name, &v.SavedAt, &v.Author, &v.ViaAI, &v.Text)
	if errors.Is(err, sql.ErrNoRows) {
		return v, errNotFound
	}
	v.Size = len(v.Text)
	return v, err
}

// versionedDoc loads a document the user may see AND edit (history is for editors only).
func (a *App) versionedDoc(u *User, id int64) (Document, error) {
	d, err := a.accessibleDoc(u, id)
	if err != nil {
		return d, err
	}
	if !a.canEditDoc(u, d) {
		return d, errForbidden
	}
	return d, nil
}

func (a *App) restoreDocVersion(u *User, d Document, ver int64, viaAI bool) (DocVersion, error) {
	v, err := a.docVersionText(d.ID, ver)
	if err != nil {
		return v, err
	}
	if _, err := a.writeDoc(u, d, v.Text, "", viaAI); err != nil {
		return v, err
	}
	a.liveAll("docs")
	return v, nil
}

// ---------- HTTP ----------

func (a *App) handleDocVersions(w http.ResponseWriter, r *http.Request) {
	d, err := a.versionedDoc(currentUser(r), pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	vs, err := a.docVersions(d.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, map[string]any{"current": a.docVersion(d.ID), "versions": vs})
}

func (a *App) handleDocVersion(w http.ResponseWriter, r *http.Request) {
	d, err := a.versionedDoc(currentUser(r), pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ver, _ := strconv.ParseInt(r.PathValue("v"), 10, 64)
	v, err := a.docVersionText(d.ID, ver)
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, v)
}

func (a *App) handleRestoreDocVersion(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	d, err := a.versionedDoc(u, pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ver, _ := strconv.ParseInt(r.PathValue("v"), 10, 64)
	if _, err := a.restoreDocVersion(u, d, ver, false); err != nil {
		if errors.Is(err, errNotFound) {
			writeErr(w, err)
		} else {
			jsonErr(w, 400, err.Error())
		}
		return
	}
	jsonOK(w, map[string]any{"ok": true, "version": a.docVersion(d.ID), "at": time.Now().Unix()})
}
