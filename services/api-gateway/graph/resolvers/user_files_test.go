package resolvers

import (
	"strings"
	"testing"
)

func TestSniffFile(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 20))
	jpg := []byte("\xff\xd8\xff\xe0" + strings.Repeat("x", 20))
	pdf := []byte("%PDF-1.7\n" + strings.Repeat("x", 20))
	docx := []byte("PK\x03\x04" + strings.Repeat("x", 10) + "word/document.xml")
	html := []byte("<html><script>alert(1)</script></html>")

	ok := []struct {
		kind, name string
		data       []byte
		want       string
	}{
		{"avatar", "me.png", png, "image/png"},
		{"avatar", "me.txt", jpg, "image/jpeg"}, // the bytes decide, not the name
		{"resume", "cv.pdf", pdf, "application/pdf"},
		{"resume", "cv.docx", docx, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	}
	for _, c := range ok {
		got, err := sniffFile(c.kind, c.data, c.name)
		if err != nil || got != c.want {
			t.Errorf("sniffFile(%s, %s) = %q, %v; want %q", c.kind, c.name, got, err, c.want)
		}
	}
	bad := []struct {
		kind, name string
		data       []byte
	}{
		{"avatar", "x.png", html},
		{"avatar", "x.pdf", pdf},
		{"resume", "cv.pdf", html},
		{"resume", "cv.pdf", png},
		{"resume", "cv.zip", docx}, // a zip is only a resume when named .docx
	}
	for _, c := range bad {
		if _, err := sniffFile(c.kind, c.data, c.name); err == nil {
			t.Errorf("sniffFile(%s, %s) accepted a bad file", c.kind, c.name)
		}
	}
}

func TestCleanFileName(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd":       "passwd",
		`C:\Users\me\cv "x".pdf`: "cv x.pdf",
		"":                       "fallback",
		"line\nbreak.pdf":        "linebreak.pdf",
		strings.Repeat("a", 300): strings.Repeat("a", maxFileName),
	}
	for in, want := range cases {
		if got := cleanFileName(in, "fallback"); got != want {
			t.Errorf("cleanFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
