package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

type Document struct {
	ID        int64   `json:"id"`
	OwnerID   int64   `json:"owner_id"`
	Owner     string  `json:"owner,omitempty"`
	Shares    []int64 `json:"shares"` // group ids; 0 = everyone
	CanEdit   bool    `json:"can_edit"`
	Version   int64   `json:"version"`
	UpdatedAt int64   `json:"updated_at"`
	Name      string  `json:"name"`
	Mime      string  `json:"mime"`
	Size      int64   `json:"size"`
	ViaAI     bool    `json:"via_ai"`
	CreatedAt int64   `json:"created_at"`
	path      string
}

func (a *App) listDocs(uid int64) ([]Document, error) {
	rows, err := a.db.Query(`SELECT id,user_id,name,mime,size,via_ai,created_at,path,version,updated_at FROM documents WHERE user_id=? ORDER BY MAX(created_at,updated_at) DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		rows.Scan(&d.ID, &d.OwnerID, &d.Name, &d.Mime, &d.Size, &d.ViaAI, &d.CreatedAt, &d.path, &d.Version, &d.UpdatedAt)
		out = append(out, d)
	}
	rows.Close()
	for i := range out {
		out[i].Shares = a.docShares(out[i].ID)
	}
	return out, nil
}

func (a *App) docShares(id int64) []int64 {
	out := []int64{}
	rows, err := a.db.Query(`SELECT group_id FROM doc_shares WHERE doc_id=? ORDER BY group_id`, id)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var g int64
		rows.Scan(&g)
		out = append(out, g)
	}
	return out
}

// docAccessSQL matches documents visible to user ? (owner, shared with everyone, or with one of their groups).
const docAccessSQL = `(d.user_id=? OR EXISTS(SELECT 1 FROM doc_shares s WHERE s.doc_id=d.id AND (s.group_id=0
	OR s.group_id IN (SELECT group_id FROM group_members WHERE user_id=?))))`

const docSelect = `SELECT d.id,d.user_id,COALESCE(NULLIF(u.name,''),u.username),d.name,d.mime,d.size,d.via_ai,d.created_at,d.path,d.version,d.updated_at
	FROM documents d JOIN users u ON u.id=d.user_id`

func scanDoc(s interface{ Scan(...any) error }) (Document, error) {
	var d Document
	err := s.Scan(&d.ID, &d.OwnerID, &d.Owner, &d.Name, &d.Mime, &d.Size, &d.ViaAI, &d.CreatedAt, &d.path, &d.Version, &d.UpdatedAt)
	return d, err
}

// accessibleDoc returns a document the user may read (own or shared with them).
func (a *App) accessibleDoc(u *User, id int64) (Document, error) {
	d, err := scanDoc(a.db.QueryRow(docSelect+` WHERE d.id=? AND `+docAccessSQL, id, u.ID, u.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return d, errNotFound
	}
	if err == nil {
		d.Shares = a.docShares(d.ID)
	}
	return d, err
}

// sharedDocs lists other users' documents shared with u. group>=0 restricts to one share target.
func (a *App) sharedDocs(u *User, group int64) ([]Document, error) {
	q := docSelect + ` WHERE d.user_id<>? AND ` + docAccessSQL
	args := []any{u.ID, u.ID, u.ID}
	if group >= 0 {
		// own documents are already in the "mine" list: never list them twice
		q = docSelect + ` WHERE d.user_id<>? AND d.id IN (SELECT doc_id FROM doc_shares WHERE group_id=?) AND ` + docAccessSQL
		args = []any{u.ID, group, u.ID, u.ID}
	}
	rows, err := a.db.Query(q+` ORDER BY d.created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	out := []Document{}
	for rows.Next() {
		if d, err := scanDoc(rows); err == nil {
			out = append(out, d)
		}
	}
	rows.Close()
	for i := range out {
		out[i].Shares = a.docShares(out[i].ID)
	}
	return out, nil
}

func (a *App) getDoc(uid, id int64) (Document, error) {
	var d Document
	err := a.db.QueryRow(`SELECT id,user_id,name,mime,size,via_ai,created_at,path,version,updated_at FROM documents WHERE id=? AND user_id=?`, id, uid).
		Scan(&d.ID, &d.OwnerID, &d.Name, &d.Mime, &d.Size, &d.ViaAI, &d.CreatedAt, &d.path, &d.Version, &d.UpdatedAt)
	d.Shares = a.docShares(d.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return d, errNotFound
	}
	return d, err
}

func (a *App) usedSpace(uid int64) int64 {
	var n int64
	a.db.QueryRow(`SELECT COALESCE(SUM(size),0) FROM documents WHERE user_id=?`, uid).Scan(&n)
	return n
}

func cleanName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	if name == "." || name == "" {
		name = "file"
	}
	if len(name) > 160 {
		name = name[:160]
	}
	return name
}

// storeDoc saves content to the user's private space.
func (a *App) storeDoc(uid int64, name, mimeType string, r io.Reader, viaAI bool) (Document, error) {
	dir := a.userDir(uid)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Document{}, err
	}
	name = cleanName(name)
	disk := randomToken(12) + strings.ToLower(filepath.Ext(name))
	full := filepath.Join(dir, disk)
	f, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return Document{}, err
	}
	remaining := a.userQuota(uid) - a.usedSpace(uid)
	limit := min(a.cfg.MaxUpload, remaining)
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	f.Close()
	if err != nil {
		os.Remove(full)
		return Document{}, err
	}
	if n > limit {
		os.Remove(full)
		if remaining < a.cfg.MaxUpload {
			return Document{}, errors.New("quota_exceeded")
		}
		return Document{}, errors.New("file_too_large")
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
			mimeType = t
		} else {
			mimeType = "application/octet-stream"
		}
	}
	res, err := a.db.Exec(`INSERT INTO documents(user_id,name,mime,size,path,via_ai,created_at) VALUES(?,?,?,?,?,?,?)`,
		uid, name, mimeType, n, disk, viaAI, time.Now().Unix())
	if err != nil {
		os.Remove(full)
		return Document{}, err
	}
	id, _ := res.LastInsertId()
	return a.getDoc(uid, id)
}

const maxTextChars = 60000

// docText extracts readable text from a document for the AI / preview.
func (a *App) docText(d Document) (string, error) {
	full := filepath.Join(a.userDir(d.OwnerID), d.path)
	ext := strings.ToLower(filepath.Ext(d.Name))
	if ext == ".pdf" || d.Mime == "application/pdf" {
		f, r, err := pdf.Open(full)
		if err != nil {
			return "", fmt.Errorf("pdf: %w", err)
		}
		defer f.Close()
		var buf bytes.Buffer
		for i := 1; i <= r.NumPage() && buf.Len() < maxTextChars; i++ {
			p := r.Page(i)
			if p.V.IsNull() {
				continue
			}
			t, err := p.GetPlainText(nil)
			if err == nil {
				fmt.Fprintf(&buf, "\n--- page %d ---\n%s", i, t)
			}
		}
		return truncate(buf.String()), nil
	}
	f, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxTextChars*4))
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return "", errors.New("unsupported_format")
	}
	return truncate(string(b)), nil
}

func truncate(s string) string {
	if utf8.RuneCountInString(s) <= maxTextChars {
		return s
	}
	r := []rune(s)
	return string(r[:maxTextChars]) + "\n…[truncated]"
}

// ---------- HTTP ----------

func (a *App) handleListDocs(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	docs, err := a.listDocs(u.ID)
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	group := int64(-1)
	if g := r.URL.Query().Get("group"); g != "" {
		fmt.Sscan(g, &group)
		mine := []Document{}
		for _, d := range docs { // own docs shared to that target
			for _, s := range d.Shares {
				if s == group {
					d.Owner = u.Name
					mine = append(mine, d)
				}
			}
		}
		docs = mine
	}
	shared, err := a.sharedDocs(u, group)
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	a.markEditable(u, docs)
	a.markEditable(u, shared)
	jsonOK(w, map[string]any{"docs": docs, "shared": shared, "used": a.usedSpace(u.ID), "quota": a.userQuota(u.ID), "max_file": a.cfg.MaxUpload})
}

func (a *App) handleUploadDoc(w http.ResponseWriter, r *http.Request) {
	if !a.docUploadOn() {
		jsonErr(w, 403, "uploads_disabled")
		return
	}
	u := currentUser(r)
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUpload+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	var saved []Document
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			jsonErr(w, 400, "file_too_large")
			return
		}
		if part.FileName() == "" {
			continue
		}
		d, err := a.storeDoc(u.ID, part.FileName(), part.Header.Get("Content-Type"), part, false)
		if err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		saved = append(saved, d)
	}
	jsonOK(w, saved)
}

func (a *App) handleGetDoc(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	d, err := a.accessibleDoc(u, pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	f, err := os.Open(filepath.Join(a.userDir(d.OwnerID), d.path))
	if err != nil {
		jsonErr(w, 404, "not_found")
		return
	}
	defer f.Close()
	disp := "attachment"
	safeInline := strings.HasPrefix(d.Mime, "image/") && d.Mime != "image/svg+xml" || d.Mime == "application/pdf" ||
		strings.HasPrefix(d.Mime, "text/plain") || strings.HasPrefix(d.Mime, "video/") || strings.HasPrefix(d.Mime, "audio/")
	if r.URL.Query().Get("inline") == "1" && safeInline {
		disp = "inline"
	}
	w.Header().Set("Content-Type", d.Mime)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": d.Name}))
	if d.Mime != "application/pdf" { // Chrome's PDF viewer refuses to run in a CSP sandbox
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self'; style-src 'unsafe-inline'")
	}
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	http.ServeContent(w, r, "", time.Unix(d.CreatedAt, 0), f)
}

func (a *App) handleDocText(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	d, err := a.accessibleDoc(u, pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	t, err := a.docText(d)
	if err != nil {
		jsonErr(w, 415, "unsupported_format")
		return
	}
	jsonOK(w, map[string]any{"text": t, "name": d.Name, "version": d.Version, "can_edit": a.canEditDoc(u, d)})
}

func (a *App) deleteDoc(uid, id int64) (Document, error) {
	d, err := a.getDoc(uid, id)
	if err != nil {
		return d, err
	}
	a.db.Exec(`DELETE FROM documents WHERE id=?`, d.ID)
	os.Remove(filepath.Join(a.userDir(uid), d.path))
	return d, nil
}

func (a *App) handleDeleteDoc(w http.ResponseWriter, r *http.Request) {
	if _, err := a.deleteDoc(currentUser(r).ID, pathID(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// handleShareDoc replaces the share targets of one of the user's documents.
func (a *App) handleShareDoc(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	d, err := a.getDoc(u.ID, pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Groups []int64 `json:"groups"` // 0 = everyone
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if err := a.setDocShares(u, d.ID, in.Groups); err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, map[string]any{"shares": a.docShares(d.ID)})
}

// setDocShares replaces the share targets of a document owned by u (0 = everyone).
func (a *App) setDocShares(u *User, docID int64, groups []int64) error {
	for _, g := range groups {
		if g != 0 && (!a.groupExists(g) || !(u.IsAdmin() || a.isMember(u.ID, g))) {
			return errForbidden
		}
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	tx.Exec(`DELETE FROM doc_shares WHERE doc_id=?`, docID)
	for _, g := range groups {
		tx.Exec(`INSERT OR IGNORE INTO doc_shares(doc_id,group_id,created_at) VALUES(?,?,?)`, docID, g, time.Now().Unix())
	}
	err = tx.Commit()
	if err == nil {
		a.liveAll("docs")
	}
	return err
}

func (a *App) docInRoom(docID, room int64) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM doc_shares WHERE doc_id=? AND group_id IN (0,?)`, docID, room).Scan(&n)
	return n > 0
}

// roomDocs lists documents shared with a chat (its group, or everyone).
func (a *App) roomDocs(u *User, room int64) ([]Document, error) {
	rows, err := a.db.Query(docSelect+` WHERE d.id IN (SELECT doc_id FROM doc_shares WHERE group_id IN (0,?)) ORDER BY d.created_at DESC LIMIT 300`, room)
	if err != nil {
		return nil, err
	}
	out := []Document{}
	for rows.Next() {
		if d, err := scanDoc(rows); err == nil {
			out = append(out, d)
		}
	}
	rows.Close()
	for i := range out {
		out[i].Shares = a.docShares(out[i].ID)
	}
	return out, nil
}
