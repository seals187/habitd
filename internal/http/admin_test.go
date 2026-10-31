package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/model"
)

// adminPaths lists every page of the admin area.
var adminPaths = []string{"/admin", "/admin/users", "/admin/system", "/admin/badges", "/admin/badges/new", "/admin/badges/funke/edit", "/admin/badges/mitternacht/edit",
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
