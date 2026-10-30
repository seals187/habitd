package templates

import (
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/web"
)

var knownTags = []string{"", "alkohol", "anrufen", "auto", "budget", "doomscroll", "einweg", "fleisch", "fokus", "frueh",
	"geburtstag", "handy", "idee", "impulskauf", "lesen", "lieferdienst", "meditation", "mentor", "schlaf", "schritte",
	"social", "sport", "velo", "verschwendung", "wasser", "weiterbildung"}

func TestTemplatesAreValid(t *testing.T) {
	sprite, err := web.FS.ReadFile("static/icons/sprite.svg")
	if err != nil {
		t.Fatal(err)
	}
	icons := map[string]bool{}
	for _, m := range regexp.MustCompile(`<symbol id="([^"]+)"`).FindAllStringSubmatch(string(sprite), -1) {
		icons[m[1]] = true
	}
	areas := map[string]bool{}
	for _, a := range Areas() {
		areas[a.ID] = true
		if !icons[a.Icon] {
			t.Errorf("area %s: unknown icon %s", a.ID, a.Icon)
		}
	}
	if len(areas) != 10 {
		t.Fatalf("areas: %d", len(areas))
	}
	if len(All()) != 200 || CountByKind(model.KindBuild) != 100 || CountByKind(model.KindAvoid) != 100 {
		t.Fatalf("templates: %d", len(All()))
	}
	seen := map[string]bool{}
	for _, x := range All() {
		if x.ID == "" || seen[x.ID] {
			t.Errorf("duplicate or empty id %q", x.ID)
		}
		seen[x.ID] = true
		if !areas[x.Area] {
			t.Errorf("%s: unknown area %s", x.ID, x.Area)
		}
		if !icons[x.Icon] {
			t.Errorf("%s: unknown icon %s", x.ID, x.Icon)
		}
		if !slices.Contains(knownTags, x.Tag) {
			t.Errorf("%s: unknown tag %s", x.ID, x.Tag)
		}
		if x.Kind != model.KindBuild && x.Kind != model.KindAvoid {
			t.Errorf("%s: kind %s", x.ID, x.Kind)
		}
		if x.Difficulty != model.Easy && x.Difficulty != model.Medium && x.Difficulty != model.Hard {
			t.Errorf("%s: difficulty %s", x.ID, x.Difficulty)
		}
		switch x.Frequency.Type {
		case "daily":
		case "weekly", "monthly":
			if x.Frequency.Times < 1 {
				t.Errorf("%s: times %d", x.ID, x.Frequency.Times)
			}
		default:
			t.Errorf("%s: frequency %s", x.ID, x.Frequency.Type)
		}
	}
}

func TestSearchToleratesCaseAndUmlauts(t *testing.T) {
	for _, q := range []string{"GLÄSER", "glaeser", "glaser"} {
		got := Filter(q, "", "")
		if len(got) == 0 || got[0].ID != "8-glaser-wasser-trinken" {
			t.Errorf("%q: %+v", q, got)
		}
	}
}

func TestSuggestLimitsAndPrefersPopular(t *testing.T) {
	got := Suggest("Sp", 5)
	if len(got) != 5 || !got[0].Popular {
		t.Fatalf("got %+v", got)
	}
	if Suggest("  ", 5) != nil {
		t.Fatal("empty query must not suggest")
	}
}

func TestFilters(t *testing.T) {
	if n := len(Filter("", model.KindAvoid, "")); n != 100 {
		t.Fatalf("avoid: %d", n)
	}
	if n := len(Filter("", "", "digital")); n != 16 {
		t.Fatalf("digital: %d", n)
	}
	if n := CountByArea()["gesundheit"]; n != 40 {
		t.Fatalf("gesundheit: %d", n)
	}
}

func TestStartersShowAtLeastTwo(t *testing.T) {
	for _, a := range Areas() {
		got := Starters(a.ID, 3)
		if len(got) < 2 || len(got) > 3 {
			t.Errorf("%s: %d", a.ID, len(got))
		}
	}
}

func TestHabitKeepsTagAndRhythm(t *testing.T) {
	x, ok := Find("joggen")
	if !ok {
		t.Fatal("joggen missing")
	}
	h := x.Habit(3, time.Now())
	if h.Tag != "sport" || h.Frequency != model.FreqWeekly || h.WeeklyTarget != 3 || h.TemplateID != "joggen" || h.AreaID != 3 {
		t.Fatalf("got %+v", h)
	}
}
