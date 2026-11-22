package resolvers

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Run with -update-golden to rewrite testdata/emails after an intended change
// to an email; otherwise every email must render byte-for-byte as recorded.
var updateGolden = flag.Bool("update-golden", false, "rewrite the golden email files")

func TestEmailsMatchGolden(t *testing.T) {
	expires := time.Date(2026, 11, 2, 18, 30, 0, 0, time.UTC)
	program := referralProgram{IsActive: true, CourseRewardPaise: 50000, ExamRewardPaise: 20000,
		FriendDiscountPercent: 10, FriendDiscountCap: 150000, AttributionDays: 30, TermsURL: "https://example.com/terms"}
	// Names carry markup so escaping is part of what is pinned.
	emails := map[string]string{
		"certification_link.html":             certificationLinkHTML("Asha <b>K</b>", "Java Certification", "https://example.com/c?t=a&b=1", "2 Nov 2026"),
		"referral_link.html":                  referralLinkHTML("Ravi & Co", "https://example.com/r/KNOV123", "KNOV123", program),
		"referral_earned.html":                referralEarnedHTML("Ravi", "Meera", "Full Stack Engineering", 500),
		"hiring_invite.html":                  hiringInviteHTML("Asha", "Acme <Labs>", "Backend Test", "https://example.com/h?t=x", 60, 40, expires),
		"scholarship_applicant_new.html":      applicantHTML("Asha", "Full Stack", "Scholarship Test", "https://example.com/s?t=1", 90, 100, true),
		"scholarship_applicant_existing.html": applicantHTML("Asha", "Full Stack", "Scholarship Test", "https://example.com/s?t=1", 90, 100, false),
		"scholarship_award.html":              awardHTML("Asha", "Full Stack", 40),
	}
	for name, got := range emails {
		path := filepath.Join("testdata", "emails", name)
		if *updateGolden {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run with -update-golden to create it)", name, err)
		}
		if got != string(want) {
			t.Errorf("%s no longer matches %s", name, path)
		}
	}
}
