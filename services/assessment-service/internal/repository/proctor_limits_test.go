package repository

import (
	"strings"
	"testing"
)

func TestCleanProctorKind(t *testing.T) {
	cases := map[string]string{
		"tab_blur":                      "tab_blur",
		"Fullscreen_Exit":               "fullscreen_exit",
		"<script>alert(1)</script>":     "scriptalert1script",
		"candidate cleared by reviewer": "candidateclearedbyreviewer",
		"":                              "",
		"!!!":                           "",
		strings.Repeat("k", 5000):       strings.Repeat("k", maxProctorKind),
	}
	for in, want := range cases {
		if got := cleanProctorKind(in); got != want {
			t.Errorf("cleanProctorKind(%.30q) = %q, want %q", in, got, want)
		}
	}
}

func TestClipRunes(t *testing.T) {
	if got := clipRunes("⌘⇧4", 2); got != "⌘⇧" {
		t.Errorf("clipRunes kept a broken sequence: %q", got)
	}
	if got := clipRunes(strings.Repeat("A", 200000), maxProctorDetail); len(got) != maxProctorDetail {
		t.Errorf("clipRunes length = %d, want %d", len(got), maxProctorDetail)
	}
	if got := clipRunes("short", maxProctorDetail); got != "short" {
		t.Errorf("clipRunes changed a short string: %q", got)
	}
}
