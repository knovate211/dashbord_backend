//go:build integration

package resolvers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// Run with: KFILES_DSN=postgres://... go test -tags integration -run TestUserFilesFlow ./graph/resolvers/
func TestUserFilesFlow(t *testing.T) {
	dsn := os.Getenv("KFILES_DSN")
	if dsn == "" {
		t.Skip("KFILES_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	h, err := NewUserFilesHandler(ctx, pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	user := "00000000-0000-0000-0000-00000000f11e"
	other := "00000000-0000-0000-0000-00000000beef"
	defer pool.Exec(ctx, `DELETE FROM user_files WHERE user_id IN ($1, $2)`, user, other)

	call := func(method, path, uid, role string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		ctx := context.WithValue(r.Context(), middleware.CtxUserID, uid)
		r = r.WithContext(context.WithValue(ctx, middleware.CtxRole, role))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	pdf := []byte("%PDF-1.4\n%test resume\n")
	if w := call("PUT", "/api/files/me/resume?name=My%20CV.pdf", user, "student", pdf); w.Code != 200 {
		t.Fatalf("upload resume: %d %s", w.Code, w.Body)
	}
	if w := call("PUT", "/api/files/me/resume?name=x.pdf", user, "student", []byte("<html><script>")); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("html as resume: %d", w.Code)
	}
	big := append([]byte("%PDF-"), bytes.Repeat([]byte("x"), maxResumeBytes)...)
	if w := call("PUT", "/api/files/me/resume?name=big.pdf", user, "student", big); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too big: %d", w.Code)
	}
	w := call("GET", "/api/files/me", user, "student", nil)
	var meta map[string]*fileMeta
	_ = json.Unmarshal(w.Body.Bytes(), &meta)
	if meta["resume"] == nil || meta["resume"].Name != "My CV.pdf" || meta["avatar"] != nil {
		t.Fatalf("meta: %s", w.Body)
	}
	w = call("GET", "/api/files/me/resume", user, "student", nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pdf) || w.Header().Get("Content-Type") != "application/pdf" ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("download: %d %v", w.Code, w.Header())
	}
	// Someone else cannot read it; a non-admin cannot use the admin route; an admin can.
	if w := call("GET", "/api/files/me/resume", other, "student", nil); w.Code != 404 {
		t.Fatalf("other user read: %d", w.Code)
	}
	if w := call("GET", "/api/files/user/"+user+"/resume", other, "student", nil); w.Code != 403 {
		t.Fatalf("student on admin route: %d", w.Code)
	}
	if w := call("GET", "/api/files/user/"+user+"/resume", other, "admin", nil); w.Code != 200 {
		t.Fatalf("admin read: %d", w.Code)
	}
	if w := call("GET", "/api/files/me", "", "", nil); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 40))
	if w := call("PUT", "/api/files/me/avatar", user, "student", png); w.Code != 200 {
		t.Fatalf("avatar: %d %s", w.Code, w.Body)
	}
	if w := call("DELETE", "/api/files/me/resume", user, "student", nil); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if w := call("GET", "/api/files/me/resume", user, "student", nil); w.Code != 404 {
		t.Fatalf("after delete: %d", w.Code)
	}
}
