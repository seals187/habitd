package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/model"
)

func badgeValues(c *client) url.Values {
	return url.Values{"csrf": {c.csrf}, "name": {"Hundert Seiten"}, "description": {"Lies hundert Seiten."}, "category": {"bereiche"}, "icon": {"book"},
		"color": {""}, "rule": {"count"}, "scope": {"any_habit"}, "target": {"3"}, "coins": {"20"}, "jokers": {"1"}, "sort": {"200"}}
}

func TestBadgeListFilters(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	_, html := a.get("/admin/badges")
	for _, want := range []string{"95 eingebaut · 0 eigene · 0 deaktiviert", "Funke", "Serie ≥ 3 Tage", "Check-ins ≥ 1", "Bereich Gute Taten · Anzahl ≥ 20", "Tag doomscroll · Serie ≥ 30 Tage", "Vermeiden-Serie ≥ 30", "+50"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	_, html = a.get("/admin/badges?cat=level")
	if !strings.Contains(html, "Aufsteiger") || strings.Contains(html, "Funke") {
		t.Fatal("category filter")
	}
	_, html = a.do("GET", "/admin/badges?q=monatsfl", nil, map[string]string{"HX-Request": "true"})
	if strings.Contains(html, "<html") || !strings.Contains(html, "Monatsflamme") || strings.Contains(html, "Funke") {
		t.Fatalf("search: %s", html)
	}
	_, html = a.do("GET", "/admin/badges?status=geheim", nil, map[string]string{"HX-Request": "true"})
	if !strings.Contains(html, "Mitternacht") || strings.Contains(html, "Funke") {
		t.Fatal("status filter")
	}
}

func TestBadgeCreateEvaluateDelete(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	form := badgeValues(a)
	if resp, body := a.do("POST", "/admin/badges/new", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create: %s %s", resp.Status, body)
	}
	b, ok := a.svc.Rules().BadgeByKey("eigen-hundert-seiten")
	if !ok || b.Builtin || b.Hint != "3 Check-ins" || b.Coins != 20 || !b.Active {
		t.Fatalf("badge: %+v", b)
	}
	_, html := a.get("/admin/badges")
	if !strings.Contains(html, "95 eingebaut · 1 eigene") {
		t.Fatal("counts")
	}
	_, html = a.get("/admin/badges?cat=_eigene")
	if !strings.Contains(html, "Hundert Seiten") || strings.Contains(html, "Funke") {
		t.Fatal("filter eigene")
	}
	_, html = a.get("/badges?cat=bereiche")
	if !strings.Contains(html, "Lies hundert Seiten.") {
		t.Fatal("the app must show it at once")
	}
	if resp, _ := a.post("/admin/badges/eigen-hundert-seiten/delete", false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete: %s", resp.Status)
	}
	if _, ok := a.svc.Rules().BadgeByKey("eigen-hundert-seiten"); ok {
		t.Fatal("still there")
	}
}

func TestBuiltinBadgesCannotBeDeleted(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	if resp, _ := a.post("/admin/badges/funke/delete", false); resp.StatusCode == http.StatusSeeOther {
		t.Fatal("deleted a built-in badge")
	}
	if _, ok := a.svc.Rules().BadgeByKey("funke"); !ok {
		t.Fatal("funke is gone")
	}
}

func TestBadgeEditKeepsBuiltinAndChangesRule(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	_, html := a.get("/admin/badges/durchstarter/edit")
	if !strings.Contains(html, "x-data=\"badgeEditor\"") || strings.Contains(html, ">Löschen<") || !strings.Contains(html, "Deaktivieren") {
		t.Fatal("edit form")
	}
	form := badgeValues(a)
	form.Set("name", "Dauerläufer")
	form.Set("rule", "streak")
	form.Set("scope", "habit_tag")
	form.Set("tag", " Sport ")
	form.Set("target", "60")
	form.Set("count_jokers", "on")
	form.Set("color", "#F2994A")
	if resp, body := a.do("POST", "/admin/badges/durchstarter/edit", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s %s", resp.Status, body)
	}
	b, _ := a.svc.Rules().BadgeByKey("durchstarter")
	if !b.Builtin || b.Name != "Dauerläufer" || b.Metric.Value != "sport" || !b.Metric.Jokers || b.Formula() != "streak(tag: sport) >= 60" {
		t.Fatalf("badge: %+v %s", b, b.Formula())
	}
}

func TestSpecialBadgeKeepsItsLogic(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	before, _ := a.svc.Rules().BadgeByKey("mitternacht")
	form := badgeValues(a)
	form.Set("name", "Geisterstunde")
	form.Set("category", "geheim")
	form.Set("icon", "moon")
	form.Set("rule", "count")
	form.Set("target", "99")
	form.Set("secret", "on")
	if resp, body := a.do("POST", "/admin/badges/mitternacht/edit", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s %s", resp.Status, body)
	}
	b, _ := a.svc.Rules().BadgeByKey("mitternacht")
	if b.Name != "Geisterstunde" || b.Target != before.Target || b.Formula() != before.Formula() {
		t.Fatalf("badge: %+v", b)
	}
	if _, html := a.get("/admin/badges/mitternacht/edit"); !strings.Contains(html, "steckt im Code") {
		t.Fatal("no hint on the form")
	}
}

func TestBadgeToggleAndUserView(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	// A toggle that is switched off sends no value at all.
	resp, _ := a.do("POST", "/admin/badges/zehner/active", url.Values{"csrf": {a.csrf}}, map[string]string{"HX-Request": "true"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("toggle: %s", resp.Status)
	}
	if b, _ := a.svc.Rules().BadgeByKey("zehner"); b.Active {
		t.Fatal("still active")
	}
	if _, html := a.get("/badges?cat=meilensteine"); strings.Contains(html, "Zehner") {
		t.Fatal("deactivated badge shown to someone who does not hold it")
	}
	a.do("POST", "/admin/badges/zehner/active", url.Values{"csrf": {a.csrf}, "active": {"on"}}, map[string]string{"HX-Request": "true"})
	if b, _ := a.svc.Rules().BadgeByKey("zehner"); !b.Active {
		t.Fatal("not back")
	}
}

func TestBadgeValidation(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	for field, value := range map[string]string{"name": "", "category": "gibtsnicht", "icon": "gibtsnicht", "color": "red", "rule": "magie", "scope": "magie", "target": "0", "coins": "-1", "jokers": "9"} {
		form := badgeValues(a)
		form.Set(field, value)
		if resp, _ := a.do("POST", "/admin/badges/new", form, nil); resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%s=%q: %s", field, value, resp.Status)
		}
	}
	form := badgeValues(a)
	form.Set("scope", "habit_tag")
	if resp, _ := a.do("POST", "/admin/badges/new", form, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("tag missing: %s", resp.Status)
	}
}

func TestCategories(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	form := url.Values{"csrf": {a.csrf}, "name": {"Reisen"}, "color": {"#112233"}}
	if resp, _ := a.do("POST", "/admin/badges/categories", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add: %s", resp.Status)
	}
	if c, ok := a.svc.Rules().CategoryByKey("reisen"); !ok || c.Color != "#112233" {
		t.Fatalf("category: %+v", c)
	}
	resp, _ := a.post("/admin/badges/categories/serien/delete", false)
	if !strings.Contains(resp.Header.Get("Location"), "error=") {
		t.Fatal("deleted a category with badges")
	}
	a.post("/admin/badges/categories/reisen/delete", false)
	if _, ok := a.svc.Rules().CategoryByKey("reisen"); ok {
		t.Fatal("empty category stayed")
	}
}

func TestBadgeExportImport(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	a.do("POST", "/admin/badges/new", badgeValues(a), nil)
	resp, body := a.get("/admin/badges/export")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"key": "funke"`) || !strings.Contains(body, "eigen-hundert-seiten") {
		t.Fatalf("export: %s", resp.Status)
	}
	if err := a.q().SetBadgeActive("funke", false); err != nil {
		t.Fatal(err)
	}
	a.post("/admin/badges/eigen-hundert-seiten/delete", false)

	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	_ = mp.WriteField("csrf", a.csrf)
	part, _ := mp.CreateFormFile("file", "badges.json")
	_, _ = io.WriteString(part, body)
	_ = mp.Close()
	req, _ := http.NewRequest("POST", a.base+"/admin/badges/import", &buf)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	res, err := a.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.Header.Get("Location") != "/admin/badges" {
		t.Fatalf("import: %s %q", res.Status, res.Header.Get("Location"))
	}
	rules := a.svc.Rules()
	if b, _ := rules.BadgeByKey("funke"); !b.Active || !b.Builtin {
		t.Fatalf("funke: %+v", b)
	}
	if _, ok := rules.BadgeByKey("eigen-hundert-seiten"); !ok {
		t.Fatal("custom badge not restored")
	}
	if b, _ := rules.BadgeByKey("mitternacht"); b.Metric.Rule != "special" || !b.Secret {
		t.Fatalf("special badge: %+v", b)
	}
}

func TestBadgeRoutesAreClosed(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	for _, path := range []string{"/admin/badges/funke/active", "/admin/badges/new", "/admin/badges/funke/edit", "/admin/badges/categories", "/admin/badges/import"} {
		if resp, _ := b.post(path, false); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %s", path, resp.Status)
		}
	}
}
