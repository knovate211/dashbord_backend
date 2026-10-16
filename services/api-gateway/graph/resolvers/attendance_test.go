package resolvers

import (
	"net/http"
	"testing"
	"time"
)

// Monday 15 Sep 2025 — a fixed day the tests stand on.
func at(hhmm string) time.Time {
	t, _ := time.Parse("15:04", hhmm)
	return time.Date(2025, 9, 15, t.Hour(), t.Minute(), 0, 0, classTZ)
}

func javaMWF() classSchedule {
	return classSchedule{
		ID: "java", CourseID: "1", Title: "Java", Days: []int{1, 3, 5},
		StartTime: "11:00", EndTime: "12:00", StartDate: "2025-09-01", EndDate: "2025-11-30", IsActive: true,
	}
}

func TestMarkWindowIsTheClassItself(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want int
	}{
		{"a minute before start", at("10:59"), http.StatusConflict},
		{"exactly at start", at("11:00"), 0},
		{"mid class", at("11:30"), 0},
		{"last minute", at("11:59"), 0},
		{"exactly at end", at("12:00"), http.StatusConflict},
		{"not a class day (Tuesday)", at("11:30").AddDate(0, 0, 1), http.StatusConflict},
		{"after the course ends", time.Date(2025, 12, 1, 11, 30, 0, 0, classTZ), http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, msg := checkMarkWindow(javaMWF(), tt.now); code != tt.want {
				t.Fatalf("code = %d (%q), want %d", code, msg, tt.want)
			}
		})
	}
}

// The server may run in UTC; 05:30 UTC is 11:00 in India and must count as class time.
func TestMarkWindowUsesClassTimezone(t *testing.T) {
	utc := time.Date(2025, 9, 15, 5, 45, 0, 0, time.UTC)
	if code, msg := checkMarkWindow(javaMWF(), utc); code != 0 {
		t.Fatalf("05:45 UTC is 11:15 IST, expected open; got %d %q", code, msg)
	}
}

func TestExpandDerivesStatus(t *testing.T) {
	day := dayStart(at("00:00"))
	marked := map[string]time.Time{"java|2025-09-12": at("11:05").AddDate(0, 0, -3)}

	// Fri 12 (marked), Sat–Sun nothing, Mon 15 depends on the clock.
	from := day.AddDate(0, 0, -3)
	check := func(now time.Time, wantToday string) {
		t.Helper()
		got := expand([]classSchedule{javaMWF()}, from, day, now, marked)
		if len(got) != 2 {
			t.Fatalf("want 2 sessions, got %d", len(got))
		}
		if got[0].Date != "2025-09-15" || got[0].Status != wantToday {
			t.Fatalf("today = %s/%s, want 2025-09-15/%s", got[0].Date, got[0].Status, wantToday)
		}
		if got[1].Status != "present" {
			t.Fatalf("Friday = %s, want present", got[1].Status)
		}
	}
	check(at("10:00"), "upcoming")
	check(at("11:10"), "live")
	check(at("12:00"), "absent")
}

func TestValidateSchedule(t *testing.T) {
	bad := []func(*classSchedule){
		func(s *classSchedule) { s.Days = nil },
		func(s *classSchedule) { s.Days = []int{7} },
		func(s *classSchedule) { s.EndTime = "11:00" },
		func(s *classSchedule) { s.StartTime = "11am" },
		func(s *classSchedule) { s.EndDate = "2025-08-01" },
		func(s *classSchedule) { s.Title = "  " },
		func(s *classSchedule) { s.MeetingURL = "javascript:alert(1)" },
	}
	for i, mutate := range bad {
		s := javaMWF()
		mutate(&s)
		if s.validate() == nil {
			t.Errorf("case %d: expected a validation error", i)
		}
	}
	s := javaMWF()
	s.Days = []int{5, 1, 3, 1}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	if len(s.Days) != 3 || s.Days[0] != 1 || s.Days[2] != 5 {
		t.Fatalf("days not normalised: %v", s.Days)
	}
}

// A student who joins mid-course has not missed the classes held before they joined.
func TestSessionsBeforeEnrolmentDoNotCount(t *testing.T) {
	s := javaMWF()
	s.joinedOn = "2025-09-12"
	day := dayStart(at("00:00"))
	got := expand([]classSchedule{s}, day.AddDate(0, 0, -14), day, at("13:00"), nil)
	for _, cs := range got {
		if cs.Date < "2025-09-12" {
			t.Fatalf("session on %s predates enrolment", cs.Date)
		}
	}
	if len(got) != 2 { // Fri 12 and Mon 15
		t.Fatalf("want 2 sessions, got %d", len(got))
	}
}
