package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/model"
)

// adminPaths lists every page of the admin area.
var adminPaths = []string{"/admin", "/admin/quests", "/admin/quests/new", "/admin/quests/1/edit", "/admin/quests/plan", "/admin/users", "/admin/system", "/admin/badges", "/admin/badges/new", "/admin/badges/funke/edit", "/admin/badges/mitternacht/edit",
	"/admin/badges/categories", "/admin/badges/transfer", "/admin/levels", "/admin/levels?all=1", "/admin/areas", "/admin/templates", "/admin/economy"}

func TestAdminPagesRender(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	for _, path := range adminPaths {
		resp, html := a.get(path)
		if resp.StatusCode != http.StatusOK || !strings.Contains(html, "Zurück zur App") {
			t.Fatalf("%s: %s", path, resp.Status)
		}
	}
	if _, html := a.get("/profile"); !strings.Contains(html, "Administration") {
		t.Fatal("no entry in the profile")
	}
}

func TestAdminIsClosedToUsers(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	for _, path := range adminPaths {
		if resp, _ := b.get(path); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %s", path, resp.Status)
		}
	}
	if _, html := b.get("/profile"); strings.Contains(html, "Administration") {
		t.Fatal("entry shown to a plain user")
	}
	anon := &client{t: t, base: a.base, http: &http.Client{CheckRedirect: a.http.CheckRedirect}}
	if resp, _ := anon.get("/admin"); resp.Header.Get("Location") != "/login" {
		t.Fatalf("anonymous: %s", resp.Status)
	}
}

func TestRoleChangeCountsAtOnce(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleAdmin)
	if resp, _ := b.get("/admin"); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin: %s", resp.Status)
	}
	if err := a.svc.SetRole(a.ctx(), b.uid, model.RoleUser); err != nil {
		t.Fatal(err)
	}
	if resp, _ := b.get("/admin"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("role survived in the session: %s", resp.Status)
	}
}

func TestQuestAdminAndPage(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	a.habit("Lesen", "build")
	_, html := a.get("/")
	if !strings.Contains(html, `href="/quest"`) {
		t.Fatal("no quest on Today")
	}
	_, html = a.get("/quest")
	for _, want := range []string{"Alles erledigt", "Zählt für diese Quest", "Lesen", "Quest-Serie", "&#43;50 Taler"} {
		if !strings.Contains(html, want) {
			t.Fatalf("quest page: missing %q", want)
		}
	}
	_, html = a.get("/admin/quests")
	for _, want := range []string{"8 Quests im Pool · 7 aktiv", "Früher Vogel", "Mo–Fr", "Ziehung", "Questmeister"} {
		if !strings.Contains(html, want) {
			t.Fatalf("admin: missing %q", want)
		}
	}
	form := url.Values{"csrf": {a.csrf}, "name": {"Neue Quest"}, "text": {"Vor {{uhrzeit}} Uhr"}, "icon": {"sun"}, "color": {"#F2C14E"}, "kind": {"until"}, "count": {"1"}, "until": {"09:00"},
		"percent": {"100"}, "reward_kind": {"xp"}, "reward": {"12"}, "weight": {"1"}, "days": {"0", "1"}}
	if resp, body := a.do("POST", "/admin/quests/new", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s %s", resp.Status, body)
	}
	if _, html = a.get("/admin/quests"); !strings.Contains(html, "Neue Quest") || !strings.Contains(html, "Vor 09:00 Uhr") || !strings.Contains(html, "Mo, Di") {
		t.Fatal("new quest not listed")
	}
	form.Set("reward", "0")
	if resp, _ := a.do("POST", "/admin/quests/new", form, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid: %s", resp.Status)
	}
	if resp, _ := a.do("POST", "/admin/quests/settings", url.Values{"csrf": {a.csrf}, "per_day": {"2"}, "rollover": {"05:00"}, "mode": {"equal"}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("settings: %s", resp.Status)
	}
	if c := a.svc.Rules().QuestConfig; c.PerDay != 2 || c.Rollover != "05:00" || c.Weighted || c.NoRepeat {
		t.Fatalf("config: %+v", c)
	}
	date := a.svc.Today().String()
	if resp, _ := a.do("POST", "/admin/quests/plan", url.Values{"csrf": {a.csrf}, "date": {date}, "quest": {"2"}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("plan: %s", resp.Status)
	}
	event := url.Values{"csrf": {a.csrf}, "name": {"Gute-Taten-Woche"}, "start": {date}, "end": {date}, "quest": {"1"}, "multiplier": {"2"}}
	if resp, _ := a.do("POST", "/admin/quests/events", event, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("event: %s", resp.Status)
	}
	if _, html = a.get("/admin/quests/plan?week=" + date); !strings.Contains(html, "fest geplant") || !strings.Contains(html, "Gute-Taten-Woche") || !strings.Contains(html, "doppelte Belohnung") {
		t.Fatalf("plan page: %s", html)
	}
	b := a.second("bert", model.RoleUser)
	for _, path := range []string{"/admin/quests/new", "/admin/quests/1/active", "/admin/quests/1/delete", "/admin/quests/plan", "/admin/quests/events"} {
		if resp, _ := b.post(path, false); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %s", path, resp.Status)
		}
	}
	if resp, _ := a.post("/admin/quests/1/delete", false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete: %s", resp.Status)
	}
}
