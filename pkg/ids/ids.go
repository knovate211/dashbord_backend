// Package ids holds identifier checks shared by the services.
package ids

// IsUUID reports whether s is a UUID in the canonical 8-4-4-4-12 hex form.
//
// Queries compare uuid columns directly (`id = $1`) so Postgres can use the
// index. Postgres rejects a malformed value with an error rather than matching
// nothing, so callers check with IsUUID first and treat a bad id as not found —
// the behaviour the older `id::text = $1` form gave, without the table scan.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
				return false
			}
		}
	}
	return true
}
