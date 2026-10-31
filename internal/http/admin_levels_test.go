package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// levelForm posts what the page shows, like a browser that changed nothing.
func levelForm(c *client) url.Values {
	rules := c.svc.Rules()
	st := rules.Settings
	form := url.Values{"csrf": {c.csrf}, "base": {decimal(st.Base)}, "exponent": {decimal(st.Exponent)}, "tempo": {st.Tempo},
		"celebrate": {"on"}, "no_drop": {"on"}, "show_title": {"on"}}
	for _, l := range rules.Levels {
		n := strconv.Itoa(l.Level)
		form.Add("level", n)
		form.Set("title_"+n, l.Title)
		form.Set("xp_"+n, strconv.Itoa(l.XP))
		form.Set("coins_"+n, strconv.Itoa(l.Coins))
		form.Set("jokers_"+n, strconv.Itoa(l.Jokers))
	}
	return form
}

func TestLevelsPageShowsTheDesign(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	_, html := a.get("/admin/levels")
	for _, want := range []string{"50 Stufen", "Einsteiger", "Alle anzeigen", "41 weitere Stufen", "XP-Kurve", "Level-Up-Feier anzeigen", "<polyline", "Ø Zeit bis Level 10"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if _, html := a.get("/admin/levels?all=1"); !strings.Contains(html, `name="title_37"`) {
		t.Fatal("all levels must open up")
	}
}

func TestLevelsSaveWithoutChangesKeepsEverything(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	before := a.svc.Rules()
	if resp, body := a.do("POST", "/admin/levels", levelForm(a), nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s %s", resp.Status, body)
	}
	after := a.svc.Rules()
	for i, l := range before.Levels {
		if after.Levels[i] != l {
			t.Fatalf("level %d changed: %+v", l.Level, after.Levels[i])
		}
	}
}

func TestLevelsOverrideAndFormula(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	form := levelForm(a)
	form.Set("title_3", "Neuer Titel")
	form.Set("xp_3", strconv.Itoa(a.svc.Rules().Levels[2].XP+1))
	form.Set("coins_3", "7")
	form.Set("base", "30")
	if resp, body := a.do("POST", "/admin/levels", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s %s", resp.Status, body)
	}
	rules := a.svc.Rules()
	l := rules.Levels[2]
	if !l.Overridden || l.Title != "Neuer Titel" || l.Coins != 7 {
		t.Fatalf("level 3: %+v", l)
	}
	if rules.Levels[9].Overridden || rules.Levels[9].XP != rules.Settings.XP(10) {
		t.Fatalf("level 10 must follow the formula: %+v", rules.Levels[9])
	}
	if _, html := a.get("/admin/levels"); !strings.Contains(html, "Zurück zur Formel") {
		t.Fatal("no mark for the override")
	}
	if resp, _ := a.post("/admin/levels/3/formula", false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("formula: %s", resp.Status)
	}
	rules = a.svc.Rules()
	if rules.Levels[2].Overridden || rules.Levels[2].XP != rules.Settings.XP(3) {
		t.Fatalf("level 3 not back: %+v", rules.Levels[2])
	}
}

func TestLevelsRejectFallingThresholds(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	form := levelForm(a)
	form.Set("xp_5", strconv.Itoa(a.svc.Rules().Levels[3].XP))
	resp, html := a.do("POST", "/admin/levels", form, nil)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(html, "streng steigen") {
		t.Fatalf("%s", resp.Status)
	}
	if a.svc.Rules().Levels[4].Overridden {
		t.Fatal("stored despite the error")
	}
	if !strings.Contains(html, `name="xp_9"`) {
		t.Fatal("levels without a milestone stay hidden on an error")
	}
}

func TestLevelsResetAndTheProfileSwitch(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	form := levelForm(a)
	form.Del("show_title")
	form.Set("title_1", "Frischling")
	if resp, _ := a.do("POST", "/admin/levels", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s", resp.Status)
	}
	if _, html := a.get("/profile"); strings.Contains(html, "Frischling") {
		t.Fatal("title shown although it is switched off")
	}
	if resp, _ := a.post("/admin/levels/reset", false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("reset: %s", resp.Status)
	}
	if r := a.svc.Rules(); r.Levels[0].Title != game.DefaultLevels()[0].Title || !r.Settings.ShowTitle {
		t.Fatalf("reset: %+v", r.Settings)
	}
}

func TestLevelsAreClosedAndProtected(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	for _, path := range []string{"/admin/levels", "/admin/levels/reset", "/admin/levels/3/formula"} {
		if resp, _ := b.post(path, false); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %s", path, resp.Status)
		}
	}
	form := levelForm(a)
	form.Del("csrf")
	if resp, _ := a.do("POST", "/admin/levels", form, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without csrf: %s", resp.Status)
	}
}

func TestEstimateLabel(t *testing.T) {
	for days, want := range map[int]string{5: "≈ 5 Tage", 21: "≈ 3 Wochen", 90: "≈ 3 Monate"} {
		if got := estimateLabel(days); got != want {
			t.Errorf("%d: %s", days, got)
		}
	}
}
