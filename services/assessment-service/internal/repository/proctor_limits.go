package repository

const (
	// maxProctorDetail bounds the free-text note on one event. The client's
	// longest real detail ("offline for 1234s", "1280px of 1920px") is tiny.
	maxProctorDetail = 300
	// maxProctorKind bounds the event name; real ones are short snake_case.
	maxProctorKind = 40
	// maxProctorEventsPerAttempt stops one attempt flooding the ledger. Past
	// it, only events that count toward the tab-switch limit are kept.
	maxProctorEventsPerAttempt = 500
)

// cleanProctorKind keeps an event name to lower-case letters, digits and
// underscores, so a tampered client cannot write text that reads like a
// verdict in the reviewer's view. It returns "" for a name with nothing left.
func cleanProctorKind(kind string) string {
	out := make([]byte, 0, len(kind))
	for i := 0; i < len(kind) && len(out) < maxProctorKind; i++ {
		c := kind[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
			out = append(out, c)
		case c >= 'A' && c <= 'Z':
			out = append(out, c+('a'-'A'))
		}
	}
	return string(out)
}

// clipRunes shortens s to at most n characters without splitting a UTF-8
// sequence (details carry symbols such as ⌘⇧).
func clipRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
