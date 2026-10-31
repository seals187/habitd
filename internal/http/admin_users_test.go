package server

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/model"
)

func TestAdminSeesFiguresButNoContents(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	b.habit("Sehr privates Habit", "build")
	_, html := a.get("/admin/users")
	if !strings.Contains(html, "bert") || strings.Contains(html, "Sehr privates Habit") {
		t.Fatal("user list")
	}
	if _, html := a.get(fmt.Sprintf("/habits/%d", 1)); strings.Contains(html, "Sehr privates") {
		t.Fatal("admin reads a foreign habit")
	}
}

func TestAdminCreatesUserWithRandomPassword(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	form := url.Values{"csrf": {a.csrf}, "username": {"neu"}, "role": {"user"}}
	resp, html := a.do("POST", "/admin/users", form, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(html, "Passwort für neu") {
		t.Fatalf("create: %s", resp.Status)
	}
	m := regexp.MustCompile(`id="secret-value" value="([0-9a-f]+)"`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no password: %s", html)
	}
	anon := a.fresh()
	if resp, _ := anon.do("POST", "/login", url.Values{"username": {"neu"}, "password": {m[1]}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %s", resp.Status)
	}
	if resp, _ := a.do("POST", "/admin/users", form, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("name taken: %s", resp.Status)
	}
	if _, html := a.get("/admin/users"); strings.Contains(html, m[1]) {
		t.Fatal("password shown twice")
	}
}

// fresh is an anonymous browser on the same server.
func (c *client) fresh() *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: c.t, base: c.base, svc: c.svc, http: &http.Client{Jar: jar, CheckRedirect: c.http.CheckRedirect}}
}

func TestInviteFlow(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	_, html := a.do("POST", "/admin/invites", url.Values{"csrf": {a.csrf}, "role": {"admin"}}, nil)
	m := regexp.MustCompile(`value="https?://[^/]+(/invite/[0-9a-f]{64})"`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no link: %s", html)
	}
	link := m[1]
	guest := a.fresh()
	if resp, html := guest.get(link); resp.StatusCode != http.StatusOK || !strings.Contains(html, "eingeladen") {
		t.Fatalf("form: %s", resp.Status)
	}
	if resp, body := guest.do("POST", link, url.Values{"username": {"gast"}, "password": {password}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("accept: %s %s", resp.Status, body)
	}
	u, err := a.q().UserByName("gast")
	if err != nil || !u.IsAdmin() {
		t.Fatalf("role of the invite: %+v %v", u, err)
	}
	second := a.fresh()
	if resp, _ := second.get(link); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second use: %s", resp.Status)
	}
	if resp, _ := second.do("POST", link, url.Values{"username": {"noch-wer"}, "password": {password}}, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second accept: %s", resp.Status)
	}
}

func TestInviteGuessesAreLimited(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	guest := a.fresh()
	for i := range maxLoginFails {
		if resp, _ := guest.get("/invite/falsch"); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("try %d: %s", i, resp.Status)
		}
	}
	if resp, _ := guest.get("/invite/falsch"); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("limit: %s", resp.Status)
	}
}

func TestLastAdminThroughTheHandlers(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	for path, form := range map[string]url.Values{
		"/admin/users/1/role":     {"csrf": {a.csrf}, "role": {"user"}},
		"/admin/users/1/disabled": {"csrf": {a.csrf}, "disabled": {"on"}},
		"/admin/users/1/delete":   {"csrf": {a.csrf}, "confirm": {"admin"}},
	} {
		if resp, _ := a.do("POST", path, form, nil); resp.StatusCode != http.StatusConflict {
			t.Fatalf("%s: %s", path, resp.Status)
		}
	}
	if resp, _ := a.get("/admin"); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin lost access: %s", resp.Status)
	}
}

func TestDeleteNeedsTheUsername(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	path := fmt.Sprintf("/admin/users/%d/delete", b.uid)
	if resp, _ := a.do("POST", path, url.Values{"csrf": {a.csrf}, "confirm": {"falsch"}}, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("wrong name: %s", resp.Status)
	}
	if resp, _ := a.do("POST", path, url.Values{"csrf": {a.csrf}, "confirm": {"bert"}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete: %s", resp.Status)
	}
	if resp, _ := b.get("/"); resp.Header.Get("Location") != "/login" {
		t.Fatal("session survived")
	}
}

func TestAdminResetsPassword(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	_, html := a.do("POST", fmt.Sprintf("/admin/users/%d/password", b.uid), url.Values{"csrf": {a.csrf}}, nil)
	m := regexp.MustCompile(`id="secret-value" value="([0-9a-f]+)"`).FindStringSubmatch(html)
	if m == nil {
		t.Fatal("no password")
	}
	if resp, _ := b.get("/"); resp.Header.Get("Location") != "/login" {
		t.Fatal("old session survived")
	}
	if resp, _ := a.fresh().do("POST", "/login", url.Values{"username": {"bert"}, "password": {m[1]}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %s", resp.Status)
	}
}

func TestUserRoutesAreClosed(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	for _, path := range []string{"/admin/users", "/admin/users/1/role", "/admin/users/1/disabled", "/admin/users/1/password", "/admin/users/1/delete", "/admin/invites"} {
		if resp, _ := b.post(path, false); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %s", path, resp.Status)
		}
	}
	if resp, _ := b.get("/admin/system/backup"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("backup: %s", resp.Status)
	}
}

func TestSystemPage(t *testing.T) {
	t.Setenv("HABITD_ADMIN_PASSWORD", "streng-geheim")
	t.Setenv("HABITD_API_TOKEN", "auch-geheim")
	t.Setenv("HABITD_ADMIN_USER", "anna")
	a := newApp(t, Config{Version: "9.9.9"})
	a.signIn()
	_, html := a.get("/admin/system")
	if !strings.Contains(html, "9.9.9") || !strings.Contains(html, "anna") || !strings.Contains(html, "gesetzt") {
		t.Fatalf("page: %s", html)
	}
	if strings.Contains(html, "streng-geheim") || strings.Contains(html, "auch-geheim") {
		t.Fatal("secret on the page")
	}
	resp, body := a.get("/admin/system/backup")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(body, "SQLite format 3") {
		t.Fatalf("backup: %s %.20q", resp.Status, body)
	}
}
