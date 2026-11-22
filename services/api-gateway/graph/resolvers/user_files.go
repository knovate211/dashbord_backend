package resolvers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// User files: a learner's profile photo and resume.
//
// Routes (all need a session):
//
//	GET    /api/files/me                    what the caller has uploaded (no bytes)
//	GET    /api/files/me/{avatar|resume}    the file itself
//	PUT    /api/files/me/{avatar|resume}    upload or replace (raw body; ?name= for a resume)
//	DELETE /api/files/me/{avatar|resume}
//	GET    /api/files/user/{id}/{kind}      admins only: another user's file
//
// Files are kept in Postgres. They are small and capped, which keeps the setup
// identical on a laptop, stage and production; swapping in object storage
// later only touches this file.
//
// The type is decided by the file's own bytes, never by its name or the
// Content-Type the browser sent, and every download goes out with that type,
// nosniff and a sandbox CSP, so an upload cannot run as a page.

const (
	fileKindAvatar = "avatar"
	fileKindResume = "resume"

	maxAvatarBytes = 2 << 20 // 2 MB (the app shrinks photos to 256 px first)
	maxResumeBytes = 5 << 20 // 5 MB
	maxFileName    = 120
)

type UserFilesHandler struct {
	Pool *pgxpool.Pool
	Log  *zap.Logger
}

func NewUserFilesHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) (*UserFilesHandler, error) {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS user_files (
			user_id      TEXT        NOT NULL,
			kind         TEXT        NOT NULL CHECK (kind IN ('avatar', 'resume')),
			filename     TEXT        NOT NULL DEFAULT '',
			content_type TEXT        NOT NULL,
			size_bytes   INTEGER     NOT NULL,
			data         BYTEA       NOT NULL,
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, kind)
		)`)
	if err != nil {
		return nil, err
	}
	return &UserFilesHandler{Pool: pool, Log: log}, nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

func (h *UserFilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// JSON unless a download replaces it with the file's own type.
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/files"), "/"), "/")

	switch {
	case len(parts) == 1 && parts[0] == "me" && r.Method == http.MethodGet:
		h.meta(w, r, userID)
	case len(parts) == 2 && parts[0] == "me" && validKind(parts[1]):
		switch r.Method {
		case http.MethodGet:
			h.download(w, r, userID, parts[1])
		case http.MethodPut:
			h.upload(w, r, userID, parts[1])
		case http.MethodDelete:
			h.remove(w, r, userID, parts[1])
		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case len(parts) == 3 && parts[0] == "user" && validKind(parts[2]) && r.Method == http.MethodGet:
		if middleware.RoleFromContext(r.Context()) != "admin" {
			writeErr(w, http.StatusForbidden, "admins only")
			return
		}
		if !uuidPattern.MatchString(parts[1]) {
			writeErr(w, http.StatusBadRequest, "invalid user id")
			return
		}
		h.download(w, r, parts[1], parts[2])
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

func validKind(k string) bool { return k == fileKindAvatar || k == fileKindResume }

type fileMeta struct {
	Name      string `json:"name"`
	Size      int    `json:"size"`
	Type      string `json:"type"`
	UpdatedAt string `json:"updatedAt"`
}

func (h *UserFilesHandler) meta(w http.ResponseWriter, r *http.Request, userID string) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT kind, filename, size_bytes, content_type, updated_at FROM user_files WHERE user_id = $1`, userID)
	if err != nil {
		h.Log.Error("list user files failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not load your files")
		return
	}
	defer rows.Close()
	out := map[string]*fileMeta{fileKindAvatar: nil, fileKindResume: nil}
	for rows.Next() {
		var kind string
		var m fileMeta
		var at time.Time
		if err := rows.Scan(&kind, &m.Name, &m.Size, &m.Type, &at); err != nil {
			h.Log.Error("scan user file failed", zap.Error(err))
			writeErr(w, http.StatusInternalServerError, "could not load your files")
			return
		}
		m.UpdatedAt = at.UTC().Format(time.RFC3339)
		out[kind] = &m
	}
	writeJSON(w, http.StatusOK, out)
}

// sniffFile returns the stored content type for an upload, or an error the
// user can act on. Only these types are accepted.
func sniffFile(kind string, data []byte, name string) (string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	switch kind {
	case fileKindAvatar:
		switch http.DetectContentType(data) {
		case "image/jpeg":
			return "image/jpeg", nil
		case "image/png":
			return "image/png", nil
		case "image/webp":
			return "image/webp", nil
		}
		return "", errors.New("the photo must be a JPG, PNG or WebP image")
	case fileKindResume:
		switch {
		case bytes.HasPrefix(data, []byte("%PDF-")):
			return "application/pdf", nil
		case bytes.HasPrefix(data, []byte("PK\x03\x04")) && ext == ".docx" && bytes.Contains(data, []byte("word/")):
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
		case bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) && ext == ".doc":
			return "application/msword", nil
		}
		return "", errors.New("the resume must be a PDF, DOC or DOCX file")
	}
	return "", errors.New("unknown file kind")
}

// cleanFileName keeps a display name safe to echo back and to put in a
// Content-Disposition header.
func cleanFileName(name, fallback string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`"<>:/\|?*`, r) {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." {
		out = fallback
	}
	for utf8.RuneCountInString(out) > maxFileName {
		_, size := utf8.DecodeLastRuneInString(out)
		out = out[:len(out)-size]
	}
	return out
}

func (h *UserFilesHandler) upload(w http.ResponseWriter, r *http.Request, userID, kind string) {
	limit := int64(maxAvatarBytes)
	if kind == fileKindResume {
		limit = maxResumeBytes
	}
	// The server's 15 s read timeout suits JSON calls, not a 5 MB resume on a
	// slow mobile connection, so uploads get longer. Ignored where unsupported.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(3 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(3 * time.Minute))
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("the file is too large — the limit is %d MB", limit>>20))
		return
	}
	if len(data) == 0 {
		writeErr(w, http.StatusBadRequest, "the file is empty")
		return
	}
	name := cleanFileName(r.URL.Query().Get("name"), map[string]string{fileKindAvatar: "photo", fileKindResume: "resume.pdf"}[kind])
	ctype, err := sniffFile(kind, data, name)
	if err != nil {
		writeErr(w, http.StatusUnsupportedMediaType, err.Error())
		return
	}

	var at time.Time
	err = h.Pool.QueryRow(r.Context(), `
		INSERT INTO user_files (user_id, kind, filename, content_type, size_bytes, data, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (user_id, kind) DO UPDATE SET
			filename = EXCLUDED.filename, content_type = EXCLUDED.content_type,
			size_bytes = EXCLUDED.size_bytes, data = EXCLUDED.data, updated_at = now()
		RETURNING updated_at`, userID, kind, name, ctype, len(data), data).Scan(&at)
	if err != nil {
		h.Log.Error("save user file failed", zap.Error(err), zap.String("kind", kind))
		writeErr(w, http.StatusInternalServerError, "could not save the file")
		return
	}
	writeJSON(w, http.StatusOK, fileMeta{Name: name, Size: len(data), Type: ctype, UpdatedAt: at.UTC().Format(time.RFC3339)})
}

func (h *UserFilesHandler) download(w http.ResponseWriter, r *http.Request, userID, kind string) {
	var name, ctype string
	var data []byte
	var at time.Time
	err := h.Pool.QueryRow(r.Context(),
		`SELECT filename, content_type, data, updated_at FROM user_files WHERE user_id = $1 AND kind = $2`,
		userID, kind).Scan(&name, &ctype, &data, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "no file uploaded")
		return
	}
	if err != nil {
		h.Log.Error("read user file failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not read the file")
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'")
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	w.Header().Set("Last-Modified", at.UTC().Format(http.TimeFormat))
	disp := "inline"
	if kind == fileKindResume && ctype != "application/pdf" {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disp, strings.ReplaceAll(name, `"`, "")))
	_, _ = w.Write(data)
}

func (h *UserFilesHandler) remove(w http.ResponseWriter, r *http.Request, userID, kind string) {
	if _, err := h.Pool.Exec(r.Context(), `DELETE FROM user_files WHERE user_id = $1 AND kind = $2`, userID, kind); err != nil {
		h.Log.Error("delete user file failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not delete the file")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
