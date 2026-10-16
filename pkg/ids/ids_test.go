package ids

import "testing"

func TestIsUUID(t *testing.T) {
	for s, want := range map[string]bool{
		"9f5eff97-b742-4b81-8c0b-0721ba683018": true,
		"9F5EFF97-B742-4B81-8C0B-0721BA683018": true,
		"":                                     false,
		"two-sum":                              false,
		"9f5eff97b7424b818c0b0721ba683018":     false,
		"9f5eff97-b742-4b81-8c0b-0721ba68301g": false,
		"9f5eff97-b742-4b81-8c0b-0721ba68301":  false,
		"9f5eff97+b742-4b81-8c0b-0721ba683018": false,
	} {
		if got := IsUUID(s); got != want {
			t.Errorf("IsUUID(%q) = %v, want %v", s, got, want)
		}
	}
}
