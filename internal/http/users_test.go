package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
)

// second signs in a further user on the same server.
func (c *client) second(name, role string) *client {
	c.t.Helper()
	id, err := c.svc.CreateUser(context.Background(), name, password, role)
	if err != nil {
		c.t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	o := &client{t: c.t, base: c.base, svc: c.svc, uid: id, http: &http.Client{Jar: jar, CheckRedirect: c.http.CheckRedirect}}
	resp, body := o.do("POST", "/login", url.Values{"username": {name}, "password": {password}}, nil)
	if resp.StatusCode != http.StatusSeeOther {
		c.t.Fatalf("login: %s %s", resp.Status, body)
	}
	_, html := o.get("/profile")
	m := csrfPattern.FindStringSubmatch(html)
	if m == nil {
		c.t.Fatalf("no csrf: %s", html)
	}
	o.csrf = m[1] + m[2]
	return o
}

func TestUsersCannotReachEachOthersData(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	habit := a.habit("Geheimes Habit", "build")
	a.post(fmt.Sprintf("/habits/%d/check", habit), false)
	if err := a.q().SaveReward(model.Reward{Title: "Geheime Belohnung", Mode: model.RewardCoins, Price: 1}); err != nil {
		t.Fatal(err)
	}
	rewards, _ := a.q().Rewards()
	reward := rewards[0].ID
	areas, _ := a.q().Areas()

	for _, path := range []string{"/", "/habits", "/rewards", "/export", "/api/v1/habits"} {
		if _, body := b.get(path); strings.Contains(body, "Geheim") {
			t.Fatalf("%s shows foreign data", path)
		}
	}
	for _, path := range []string{
		fmt.Sprintf("/habits/%d", habit),
		fmt.Sprintf("/habits/%d/edit", habit),
		fmt.Sprintf("/rewards/%d/edit", reward),
	} {
		if resp, _ := b.get(path); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s: %s", path, resp.Status)
		}
	}
	for _, path := range []string{
		fmt.Sprintf("/habits/%d/check", habit),
		fmt.Sprintf("/habits/%d/delete", habit),
		fmt.Sprintf("/habits/%d/archive", habit),
		fmt.Sprintf("/rewards/%d/redeem", reward),
		fmt.Sprintf("/rewards/%d/delete", reward),
		fmt.Sprintf("/rewards/%d/pause", reward),
		fmt.Sprintf("/rewards/%d/duplicate", reward),
		fmt.Sprintf("/rewards/%d/free", reward),
	} {
		if resp, _ := b.post(path, false); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("POST %s: %s", path, resp.Status)
		}
	}
	if resp, _ := b.do("POST", fmt.Sprintf("/habits/%d/edit", habit), url.Values{"csrf": {b.csrf}, "name": {"Fremd"}, "kind": {"build"}, "freq": {"daily"}, "icon": {"book"}, "area": {b.area()}}, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("edit: %s", resp.Status)
	}

	// A reward must not point at the habit of someone else.
	form := url.Values{"csrf": {b.csrf}, "title": {"Fremd"}, "mode": {"streak"}, "habit": {fmt.Sprint(habit)}, "target": {"3"}}
	if resp, _ := b.do("POST", "/rewards/new", form, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("reward on foreign habit: %s", resp.Status)
	}
	// Nor a habit into the area of someone else.
	form = url.Values{"csrf": {b.csrf}, "name": {"Fremd"}, "kind": {"build"}, "freq": {"daily"}, "icon": {"book"}, "area": {fmt.Sprint(areas[0].ID)}}
	if resp, _ := b.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("habit in foreign area: %s", resp.Status)
	}
	// Renaming or deleting a foreign area changes nothing.
	b.do("POST", "/settings/areas", url.Values{"csrf": {b.csrf}, "id": {fmt.Sprint(areas[0].ID)}, "name": {"Fremd"}, "color": {"#000000"}}, nil)
	b.do("POST", fmt.Sprintf("/settings/areas/%d/delete", areas[len(areas)-1].ID), url.Values{"csrf": {b.csrf}}, nil)
	after, _ := a.q().Areas()
	if len(after) != len(areas) || after[0].Name != areas[0].Name {
		t.Fatalf("areas changed: %v", after)
	}

	if h, err := a.q().Habit(habit); err != nil || h.Name != "Geheimes Habit" {
		t.Fatalf("habit: %+v %v", h, err)
	}
	if xp := a.xp(); xp == 0 {
		t.Fatal("xp of the owner gone")
	}
	if xp := b.xp(); xp != 0 {
		t.Fatalf("foreign xp: %d", xp)
	}
}

func TestLoginIsPerUser(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	a.second("Bert", model.RoleUser)
	anon := &client{t: t, base: a.base, svc: a.svc, http: &http.Client{CheckRedirect: a.http.CheckRedirect}}
	for name, want := range map[string]int{"bert": http.StatusSeeOther, "BERT": http.StatusSeeOther, "nobody": http.StatusUnauthorized} {
		if resp, _ := anon.do("POST", "/login", url.Values{"username": {name}, "password": {password}}, nil); resp.StatusCode != want {
			t.Fatalf("%s: %s", name, resp.Status)
		}
	}
}

func TestDisabledUserLosesAccessAtOnce(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	if err := a.svc.SetDisabled(context.Background(), b.uid, true); err != nil {
		t.Fatal(err)
	}
	if resp, _ := b.get("/"); resp.Header.Get("Location") != "/login" {
		t.Fatalf("session survived: %s", resp.Status)
	}
	resp, _ := b.do("POST", "/login", url.Values{"username": {"bert"}, "password": {password}}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login: %s", resp.Status)
	}
}

func TestSignupIsOffByDefault(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	if resp, _ := a.get("/signup"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get: %s", resp.Status)
	}
	if resp, _ := a.do("POST", "/signup", url.Values{"username": {"x"}, "password": {password}}, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("post: %s", resp.Status)
	}
}

func TestSignupCreatesPlainUser(t *testing.T) {
	a := newApp(t, Config{AllowSignup: true})
	a.signIn()
	anon := &client{t: t, base: a.base, svc: a.svc, http: &http.Client{CheckRedirect: a.http.CheckRedirect}}
	jar, _ := cookiejar.New(nil)
	anon.http.Jar = jar
	if resp, _ := anon.do("POST", "/signup", url.Values{"username": {"neu"}, "password": {password}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("signup: %s", resp.Status)
	}
	u, err := a.q().UserByName("neu")
	if err != nil || u.IsAdmin() {
		t.Fatalf("user: %+v %v", u, err)
	}
	if resp, _ := anon.get("/"); resp.Header.Get("Location") != "/onboarding" {
		t.Fatalf("new user must see the onboarding: %q", resp.Header.Get("Location"))
	}
	if resp, _ := anon.do("POST", "/signup", url.Values{"username": {"NEU"}, "password": {password}}, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("taken name: %s", resp.Status)
	}
}

func TestPasswordChangeSignsOutOtherSessions(t *testing.T) {
	a := newApp(t, Config{})
	a.signIn()
	b := a.second("bert", model.RoleUser)
	// A second device of bert.
	jar, _ := cookiejar.New(nil)
	other := &client{t: t, base: a.base, svc: a.svc, http: &http.Client{Jar: jar, CheckRedirect: a.http.CheckRedirect}}
	other.do("POST", "/login", url.Values{"username": {"bert"}, "password": {password}}, nil)
	form := url.Values{"csrf": {b.csrf}, "current": {password}, "password": {"ein-neues-passwort"}}
	if resp, _ := b.do("POST", "/settings/password", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("change: %s", resp.Status)
	}
	if resp, _ := other.get("/"); resp.Header.Get("Location") != "/login" {
		t.Fatal("old session survived")
	}
	if resp, _ := a.get("/profile"); resp.StatusCode != http.StatusOK {
		t.Fatalf("other users stay signed in: %s", resp.Status)
	}
}

func TestMinPasswordKept(t *testing.T) {
	if service.MinPasswordLen != 8 {
		t.Fatal("the forms promise 8 characters")
	}
}
