package resolvers

import (
	"strings"
	"testing"
)

// getMyCourses lists programmes in programOrder, so a programme missing from
// it would silently disappear from every learner's course list.
func TestProgramOrderCoversEveryProgramme(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range programOrder {
		if seen[id] {
			t.Fatalf("programme %q listed twice", id)
		}
		seen[id] = true
		if _, ok := programModules[id]; !ok {
			t.Fatalf("programOrder lists %q, which has no modules", id)
		}
	}
	for id := range programModules {
		if !seen[id] {
			t.Fatalf("programme %q is missing from programOrder", id)
		}
	}
}

func courseTitles(granted map[string]bool) []string {
	var titles []string
	for _, c := range coursesFor(granted) {
		titles = append(titles, c["title"].(string))
	}
	return titles
}

func TestCoursesForExpandsFullStackInPlace(t *testing.T) {
	cases := []struct {
		name    string
		granted map[string]bool
		want    []string
	}{
		{"full stack alone", map[string]bool{"5": true},
			[]string{"Java Development", "Front-End Technologies", "Mastering SQL", "GenAI & Forward Deployed Engineering", "Full Stack Engineering"}},
		{"golang before full stack, testing after", map[string]bool{"4": true, "5": true, "testing": true},
			[]string{"Golang Engineering", "Java Development", "Front-End Technologies", "Mastering SQL", "GenAI & Forward Deployed Engineering", "Full Stack Engineering", "Software Testing"}},
		{"java bought directly as well", map[string]bool{"1": true, "5": true},
			[]string{"Java Development", "Front-End Technologies", "Mastering SQL", "GenAI & Forward Deployed Engineering", "Full Stack Engineering"}},
		{"marketing only", map[string]bool{"seo": true},
			[]string{"SEO Fundamentals"}},
		{"nothing", map[string]bool{}, nil},
	}
	for _, c := range cases {
		got := courseTitles(c.granted)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s:\n got  %v\n want %v", c.name, got, c.want)
		}
	}
}

func TestProgramIncludesAreKnownProgrammes(t *testing.T) {
	for bundle, included := range programIncludes {
		if _, ok := programModules[bundle]; !ok {
			t.Fatalf("bundle %q has no modules", bundle)
		}
		for _, id := range included {
			if _, ok := programModules[id]; !ok {
				t.Fatalf("bundle %q includes unknown programme %q", bundle, id)
			}
		}
	}
}
