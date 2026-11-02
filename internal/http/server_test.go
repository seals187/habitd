package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/web"
)

const password = "correct-horse"

type client struct {
	uid  int64
	t    *testing.T
	base string
	http *http.Client
	csrf string
	svc  *service.Service
}

func newApp(t *testing.T, cfg Config) *client {
	t.Helper()
	ctx := context.Background()
	store, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := service.New(store, time.UTC)
	svc.Pick = func(int) int { return 0 }
	app, err := New(cfg, svc, web.FS)
	if err != nil {
		t.Fatal(err)
	}
	// The tests run on the real calendar: weekends and first days must not change the quest.
	quests := svc.Rules().QuestConfig
	quests.WeekendRule, quests.FirstRule = false, false
	if err := store.Q(ctx).SaveQuestConfig(quests); err != nil {
		t.Fatal(err)
	}
	if err := svc.LoadRules(ctx); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: ts.URL, svc: svc, http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}}
}

func (c *client) do(method, path string, form url.Values, headers map[string]string) (*http.Response, string) {
	c.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, string(data)
}

// ctx is the view of the database as the signed-in user.
func (c *client) ctx() context.Context { return db.WithUser(context.Background(), c.uid) }

func (c *client) q() *db.Q { return c.svc.Store.Q(c.ctx()) }

// area returns the id of the second area of the user.
func (c *client) area() string {
	c.t.Helper()
	areas, err := c.q().Areas()
	if err != nil || len(areas) < 2 {
		c.t.Fatalf("areas: %v %v", areas, err)
	}
	return fmt.Sprint(areas[1].ID)
}

func (c *client) get(path string) (*http.Response, string) { return c.do("GET", path, nil, nil) }

var csrfPattern = regexp.MustCompile(`X-CSRF-Token&#34;:&#34;([0-9a-f]+)|X-CSRF-Token":"([0-9a-f]+)`)

// signIn finishes setup and picks up the CSRF token.
func (c *client) signIn() {
	c.t.Helper()
	resp, _ := c.do("POST", "/setup", url.Values{"password": {password}}, nil)
	if resp.StatusCode != http.StatusSeeOther {
		c.t.Fatalf("setup: %s", resp.Status)
	}
	_, html := c.get("/profile")
	m := csrfPattern.FindStringSubmatch(html)
	if m == nil {
		c.t.Fatal("no csrf token in page")
	}
	c.csrf = m[1] + m[2]
	c.uid = 1
}

func (c *client) post(path string, htmx bool) (*http.Response, string) {
	c.t.Helper()
	headers := map[string]string{"X-CSRF-Token": c.csrf}
	if htmx {
		headers["HX-Request"] = "true"
	}
	return c.do("POST", path, url.Values{}, headers)
}

func (c *client) habit(name, kind string) int64 {
	c.t.Helper()
	form := url.Values{"csrf": {c.csrf}, "name": {name}, "kind": {kind}, "freq": {"daily"}, "difficulty": {"medium"}, "icon": {"book"}, "area": {c.area()}}
	resp, body := c.do("POST", "/habits/new", form, nil)
	if resp.StatusCode != http.StatusSeeOther {
		c.t.Fatalf("create habit: %s %s", resp.Status, body)
	}
	habits, err := c.q().Habits()
	if err != nil {
		c.t.Fatal(err)
	}
	return habits[len(habits)-1].ID
}

func TestHealthzAndHeaders(t *testing.T) {
	c := newApp(t, Config{Version: "test"})
	resp, body := c.get("/healthz")
	if resp.StatusCode != 200 || !strings.Contains(body, "ok") {
		t.Fatalf("%s %s", resp.Status, body)
	}
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "http") {
		t.Fatalf("csp: %s", csp)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control: %s", got)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Referrer-Policy") != "same-origin" {
		t.Fatalf("headers: %v", resp.Header)
	}
}

func TestAnonymousGoesToSetupThenLogin(t *testing.T) {
	c := newApp(t, Config{})
	resp, _ := c.get("/")
	if loc := resp.Header.Get("Location"); loc != "/setup" {
		t.Fatalf("location %q", loc)
	}
	if resp, _ := c.get("/api/v1/stats"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("api: %s", resp.Status)
	}
	c.signIn()
	other := &client{t: t, base: c.base, http: &http.Client{CheckRedirect: c.http.CheckRedirect}}
	resp, _ = other.get("/")
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Fatalf("location %q", loc)
	}
}

func TestSetupOnlyOnce(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	resp, _ := c.do("POST", "/setup", url.Values{"password": {"another-password"}}, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("post: %s", resp.Status)
	}
	if resp, _ := c.get("/setup"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get: %s", resp.Status)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	for _, secure := range []bool{true, false} {
		c := newApp(t, Config{CookieSecure: secure})
		resp, _ := c.do("POST", "/setup", url.Values{"password": {password}}, nil)
		cookies := resp.Cookies()
		if len(cookies) != 1 {
			t.Fatalf("cookies %v", cookies)
		}
		k := cookies[0]
		if !k.HttpOnly || k.SameSite != http.SameSiteLaxMode || k.Secure != secure {
			t.Fatalf("cookie %+v", k)
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	anon := &client{t: t, base: c.base, http: &http.Client{CheckRedirect: c.http.CheckRedirect}}
	for range maxLoginFails {
		if resp, _ := anon.do("POST", "/login", url.Values{"username": {"admin"}, "password": {"wrong"}}, nil); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("got %s", resp.Status)
		}
	}
	resp, _ := anon.do("POST", "/login", url.Values{"username": {"admin"}, "password": {password}}, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("got %s", resp.Status)
	}
}

func TestLimiterExpires(t *testing.T) {
	l := newLoginLimiter()
	now := time.Now()
	l.now = func() time.Time { return now }
	for range maxLoginFails {
		l.fail("1.2.3.4")
	}
	if !l.blocked("1.2.3.4") || l.blocked("5.6.7.8") {
		t.Fatal("block state")
	}
	now = now.Add(loginLockout + time.Second)
	if l.blocked("1.2.3.4") {
		t.Fatal("should expire")
	}
}

func TestCSRFRequired(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")
	resp, _ := c.do("POST", fmt.Sprintf("/habits/%d/check", id), url.Values{}, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("no token: %s", resp.Status)
	}
	resp, _ = c.do("POST", fmt.Sprintf("/habits/%d/check", id), url.Values{}, map[string]string{"X-CSRF-Token": "nope"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token: %s", resp.Status)
	}
}

func TestCrossOriginRejected(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	resp, _ := c.do("POST", "/logout", url.Values{}, map[string]string{"X-CSRF-Token": c.csrf, "Sec-Fetch-Site": "cross-site"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("got %s", resp.Status)
	}
}

func TestCheckFlowHTMX(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")

	// The first check-in unlocks a badge, so the celebration wins.
	resp, _ := c.post(fmt.Sprintf("/habits/%d/check", id), true)
	if resp.Header.Get("HX-Location") != "/celebrate" {
		t.Fatalf("redirect %q", resp.Header.Get("HX-Location"))
	}
	_, html := c.get("/celebrate")
	if !strings.Contains(html, "freigeschaltet") {
		t.Fatalf("celebration page: %s", html)
	}
	for {
		resp, _ := c.post("/celebrate", false)
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("got %s", resp.Status)
		}
		next, _ := c.get("/celebrate")
		if next.StatusCode == http.StatusSeeOther {
			break
		}
	}

	if _, html = c.get("/"); !strings.Contains(html, "hcard collapsed") {
		t.Fatal("a reached habit should fold")
	}

	// Undo answers with the fragment, not a full document.
	resp, html = c.post(fmt.Sprintf("/habits/%d/check", id), true)
	if resp.StatusCode != 200 || strings.Contains(html, "<html") || !strings.Contains(html, "Lesen") {
		t.Fatalf("fragment: %s %s", resp.Status, html)
	}
	if !strings.Contains(html, `aria-pressed="false"`) || strings.Contains(html, "hcard collapsed") {
		t.Fatal("habit should be open after undo")
	}
}

func TestDetailViews(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	if err := c.svc.Seed(c.ctx()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{"month": "cal-head", "year": "Quote pro Monat", "week": "Ziel erreicht"}
	for view, want := range cases {
		for id := 1; id <= 5; id++ {
			resp, html := c.get(fmt.Sprintf("/habits/%d?view=%s", id, view))
			if resp.StatusCode != 200 || !strings.Contains(html, want) {
				t.Fatalf("%s/%d: %s", view, id, resp.Status)
			}
		}
	}
	resp, html := c.get("/habits/1?view=year&span=12")
	if resp.StatusCode != 200 || !strings.Contains(html, "212 Tage erledigt") {
		t.Fatalf("year view: %s", html)
	}
	for _, m := range []string{"2026-02", "2024-02", "2020-12"} {
		if resp, _ := c.get("/habits/1?view=month&m=" + m); resp.StatusCode != 200 {
			t.Fatalf("month %s: %s", m, resp.Status)
		}
	}
	if resp, _ := c.get("/habits/1?view=month&m=garbage"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad month: %s", resp.Status)
	}
	if resp, _ := c.get("/habits/999"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: %s", resp.Status)
	}
}

func TestAllPagesRender(t *testing.T) {
	c := newApp(t, Config{Version: "1.0", Runtime: "Test"})
	c.signIn()
	if err := c.svc.Seed(c.ctx()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/habits", "/habits/new", "/habits/1/edit", "/habits/1", "/habits/1?view=year", "/habits/2?view=week", "/badges", "/badges?cat=geheim", "/rewards", "/rewards?tab=joker",
		"/rewards?tab=history", "/rewards/new", "/rewards/1/edit", "/profile", "/templates", "/templates?kind=avoid&area=geld&q=auf", "/onboarding", "/onboarding?step=2&area=familie", "/onboarding?step=3", "/habits/new?template=blut-spenden", "/settings/areas", "/settings/reminders", "/settings/joker",
		"/settings/backup", "/settings/server", "/manifest.webmanifest", "/sw.js", "/offline", "/static/css/app.css"} {
		resp, body := c.get(path)
		if resp.StatusCode != 200 {
			t.Errorf("%s: %s", path, resp.Status)
		}
		if strings.Contains(body, "ZgotmplZ") {
			t.Errorf("%s: template escaper rejected a value", path)
		}
	}
}

func TestRelapseFlow(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Kein Alkohol", "avoid")
	resp, html := c.post(fmt.Sprintf("/habits/%d/relapse", id), true)
	if resp.StatusCode != 200 || !strings.Contains(html, "Rückfall") {
		t.Fatalf("%s %s", resp.Status, html)
	}
	if resp, _ := c.post(fmt.Sprintf("/habits/%d/check", id), true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("avoid habits have no check-in: %s", resp.Status)
	}
}

func TestRedeemFlow(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	if err := c.svc.Seed(c.ctx()); err != nil {
		t.Fatal(err)
	}
	resp, html := c.post("/rewards/1/redeem", true)
	if resp.StatusCode != 200 || !strings.Contains(html, ">290<") {
		t.Fatalf("%s %s", resp.Status, html)
	}
	if resp, _ := c.post("/rewards/5/redeem", true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("too expensive: %s", resp.Status)
	}
	if !strings.Contains(html, "noch ") {
		t.Fatal("missing amount not shown")
	}
}

func TestRewardCreateValidates(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	resp, _ := c.do("POST", "/rewards/new", url.Values{"csrf": {c.csrf}, "title": {""}, "mode": {"coins"}, "price": {"10"}}, nil)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %s", resp.Status)
	}
	resp, _ = c.do("POST", "/rewards/new", url.Values{"csrf": {c.csrf}, "title": {"Kino"}, "mode": {"coins"}, "price": {"250"}, "repeatable": {"on"}}, nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("got %s", resp.Status)
	}
}

func TestExportHidesSecrets(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	_, body := c.get("/export")
	var e db.Export
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	if e.Version != 1 || strings.Contains(body, "$2a$") || strings.Contains(body, "password") || strings.Contains(body, "admin_user") {
		t.Fatalf("export leaks: %s", body)
	}
}

func TestBearerAPI(t *testing.T) {
	c := newApp(t, Config{APIToken: "s3cret"})
	c.signIn()
	id := c.habit("Lesen", "build")
	auth := map[string]string{"Authorization": "Bearer s3cret"}
	anon := &client{t: t, base: c.base, http: &http.Client{CheckRedirect: c.http.CheckRedirect}}
	if resp, _ := anon.do("GET", "/api/v1/habits", nil, map[string]string{"Authorization": "Bearer wrong"}); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: %s", resp.Status)
	}
	if resp, _ := anon.do("GET", "/profile", nil, auth); resp.StatusCode == 200 {
		t.Fatal("token must only open the API")
	}
	resp, body := anon.do("POST", fmt.Sprintf("/api/v1/habits/%d/checkins", id), url.Values{}, auth)
	if resp.StatusCode != 200 || !strings.Contains(body, `"done":true`) {
		t.Fatalf("%s %s", resp.Status, body)
	}
	_, body = anon.do("GET", "/api/v1/stats", nil, auth)
	var stats map[string]any
	if err := json.Unmarshal([]byte(body), &stats); err != nil {
		t.Fatal(err)
	}
	if stats["badges_total"] != float64(96) {
		t.Fatalf("stats %v", stats)
	}
	_, body = anon.do("GET", "/api/v1/habits", nil, auth)
	if !strings.Contains(body, `"done_today":true`) {
		t.Fatalf("habits %s", body)
	}
}

func TestAreaColorValidated(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	resp, _ := c.do("POST", "/settings/areas", url.Values{"csrf": {c.csrf}, "name": {"X"}, "color": {"red;background:url(x)"}}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %s", resp.Status)
	}
}

// A browser on plain HTTP sends no Sec-Fetch-Site, only Origin.
func TestSameOriginPostOnPlainHTTP(t *testing.T) {
	c := newApp(t, Config{})
	resp, _ := c.do("POST", "/setup", url.Values{"password": {password}}, map[string]string{"Origin": c.base})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("got %s", resp.Status)
	}
}

func TestLoginChecksUserName(t *testing.T) {
	c := newApp(t, Config{})
	resp, _ := c.do("POST", "/setup", url.Values{"username": {"julia"}, "password": {password}}, nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup: %s", resp.Status)
	}
	anon := &client{t: t, base: c.base, http: &http.Client{CheckRedirect: c.http.CheckRedirect}}
	if resp, _ := anon.do("POST", "/login", url.Values{"username": {"admin"}, "password": {password}}, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong name: %s", resp.Status)
	}
	if resp, _ := anon.do("POST", "/login", url.Values{"username": {"julia"}, "password": {"wrong-password"}}, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %s", resp.Status)
	}
	if resp, _ := anon.do("POST", "/login", url.Values{"username": {"julia"}, "password": {password}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("correct login: %s", resp.Status)
	}
}

func TestFreshInstallGoesThroughOnboarding(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	resp, _ := c.get("/")
	if loc := resp.Header.Get("Location"); loc != "/onboarding" {
		t.Fatalf("location %q", loc)
	}
	_, html := c.get("/onboarding")
	if !strings.Contains(html, "Was willst du angehen?") || !strings.Contains(html, "Digital") {
		t.Fatal("step 1")
	}
	_, html = c.get("/onboarding?step=2&area=gesundheit&area=digital")
	if !strings.Contains(html, "8 Gläser Wasser trinken") || !strings.Contains(html, "Doomscrolling") || strings.Contains(html, "Eltern anrufen") {
		t.Fatalf("step 2: %s", html)
	}
	form := url.Values{"csrf": {c.csrf}, "template": {"8-glaser-wasser-trinken", "doomscrolling", "joggen"}}
	resp, _ = c.do("POST", "/onboarding", form, nil)
	if resp.Header.Get("Location") != "/onboarding?step=3" {
		t.Fatalf("location %q", resp.Header.Get("Location"))
	}
	habits, err := c.q().Habits()
	if err != nil {
		t.Fatal(err)
	}
	if len(habits) != 3 || habits[0].Tag != "wasser" || habits[0].TemplateID == "" || habits[2].WeeklyTarget != 3 {
		t.Fatalf("habits %+v", habits)
	}
	if resp, _ := c.get("/"); resp.StatusCode != 200 {
		t.Fatalf("today: %s", resp.Status)
	}
	// Running it again must not duplicate habits.
	if resp, _ := c.do("POST", "/onboarding", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("again: %s", resp.Status)
	}
	habits, _ = c.q().Habits()
	if len(habits) != 3 {
		t.Fatalf("duplicated: %d", len(habits))
	}
}

func TestOnboardingCanBeSkipped(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	if resp, _ := c.post("/onboarding/skip", false); resp.Header.Get("Location") != "/" {
		t.Fatalf("location %q", resp.Header.Get("Location"))
	}
	if resp, _ := c.get("/"); resp.StatusCode != 200 {
		t.Fatalf("today: %s", resp.Status)
	}
}

func TestSuggestions(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	_, html := c.do("GET", "/templates/suggest?q=gl%C3%A4ser", nil, map[string]string{"HX-Request": "true"})
	if strings.Contains(html, "<html") || !strings.Contains(html, "8 Gläser Wasser trinken") || !strings.Contains(html, "als eigenes Habit anlegen") {
		t.Fatalf("suggest: %s", html)
	}
	if _, html := c.get("/templates/suggest?q="); html != "" {
		t.Fatalf("empty query: %s", html)
	}
	_, html = c.get("/habits/new?template=joggen")
	if !strings.Contains(html, `value="Joggen"`) || !strings.Contains(html, `value="joggen"`) {
		t.Fatalf("prefill: %s", html)
	}
	_, html = c.get("/habits/new?custom=1&name=Sp")
	if !strings.Contains(html, `value="Sp"`) {
		t.Fatal("custom name missing")
	}
}

func TestBoostedRequestsGetFullPages(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	full := map[string]string{"HX-Request": "true", "HX-Boosted": "true"}
	restore := map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"}
	for _, headers := range []map[string]string{full, restore} {
		if _, html := c.do("GET", "/templates", nil, headers); !strings.Contains(html, "<html") {
			t.Fatalf("%v: fragment instead of page", headers)
		}
	}
	if _, html := c.do("GET", "/templates", nil, map[string]string{"HX-Request": "true"}); strings.Contains(html, "<html") {
		t.Fatal("search got a full page")
	}
	id := c.habit("Lesen", "build")
	resp, _ := c.do("POST", fmt.Sprintf("/habits/%d/archive", id), url.Values{"csrf": {c.csrf}}, full)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("boosted form must follow a normal redirect: %s", resp.Status)
	}
}

func TestStaticCompressionAndCaching(t *testing.T) {
	c := newApp(t, Config{})
	path := "/static/css/app.css?v=1"
	resp, body := c.do("GET", path, nil, map[string]string{"Accept-Encoding": "gzip"})
	if resp.Header.Get("Content-Encoding") != "gzip" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/css") {
		t.Fatalf("headers: %v", resp.Header)
	}
	zr, err := gzip.NewReader(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	css, err := io.ReadAll(zr)
	if err != nil || !strings.Contains(string(css), "--bg:") {
		t.Fatalf("css: %v", err)
	}
	resp, body = c.do("GET", path, nil, map[string]string{"Accept-Encoding": "identity"})
	if resp.Header.Get("Content-Encoding") != "" || resp.Header.Get("Vary") != "Accept-Encoding" || !strings.Contains(body, "--bg:") {
		t.Fatalf("plain: %v", resp.Header)
	}
	resp, _ = c.get("/static/fonts/inter.woff2")
	if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("font cache: %s", resp.Header.Get("Cache-Control"))
	}
	resp, _ = c.do("GET", "/login", nil, map[string]string{"Accept-Encoding": "gzip"})
	if resp.Header.Get("Content-Encoding") != "" {
		t.Fatal("pages must stay uncompressed")
	}
}

func TestLibraryOpensTheFormFirst(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	_, html := c.get("/templates?area=umwelt&kind=avoid")
	if !strings.Contains(html, "Plastiktüten") || strings.Contains(html, "Joggen") {
		t.Fatal("filter")
	}
	_, html = c.get("/templates?q=jog")
	if !strings.Contains(html, `href="/habits/new?template=joggen"`) || strings.Contains(html, "/add") {
		t.Fatalf("a template opens the form: %s", html)
	}
	if habits, _ := c.q().Habits(); len(habits) != 0 {
		t.Fatalf("nothing is added before saving: %+v", habits)
	}
	_, html = c.get("/habits/new?template=joggen")
	if !strings.Contains(html, `value="Joggen"`) || !strings.Contains(html, `href="/templates">Abbrechen`) {
		t.Fatalf("form: %s", html)
	}
	form := url.Values{"csrf": {c.csrf}, "name": {"Joggen im Park"}, "kind": {"build"}, "template": {"joggen"}, "icon": {"heart"}, "area": {c.area()}, "difficulty": {"hard"}, "freq": {"w2"}}
	if resp, body := c.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s %s", resp.Status, body)
	}
	habits, _ := c.q().Habits()
	if len(habits) != 1 || habits[0].Tag != "sport" || habits[0].Name != "Joggen im Park" || habits[0].WeeklyTarget != 2 {
		t.Fatalf("habits %+v", habits)
	}
	if _, html = c.get("/templates?q=jog"); !strings.Contains(html, "Aktiv") {
		t.Fatal("saved template should show as active")
	}
}

func TestTemplateTagSurvivesForm(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	form := url.Values{"csrf": {c.csrf}, "name": {"Weniger scrollen"}, "kind": {"avoid"}, "template": {"doomscrolling"}, "icon": {"heart"}, "area": {c.area()}, "difficulty": {"easy"}, "freq": {"daily"}}
	if resp, _ := c.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("got %s", resp.Status)
	}
	habits, _ := c.q().Habits()
	if habits[0].Tag != "doomscroll" || habits[0].TemplateID != "doomscrolling" {
		t.Fatalf("habit %+v", habits[0])
	}
}

func TestBadgeRules(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	_, html := c.get("/badges?cat=meilensteine")
	if !strings.Contains(html, `popovertarget="rule-zehner"`) || !strings.Contains(html, "Sammle insgesamt 10 Check-ins") || !strings.Contains(html, "0 / 10") {
		t.Fatalf("rule missing: %s", html)
	}
	_, html = c.get("/badges?cat=geheim")
	if !strings.Contains(html, "Dieses Abzeichen ist geheim") || strings.Contains(html, "14. März") {
		t.Fatal("secret rule leaked")
	}
}

func (c *client) meterHabit(name, measure, target, step, unit string) int64 {
	c.t.Helper()
	form := url.Values{"csrf": {c.csrf}, "name": {name}, "kind": {"build"}, "freq": {"daily"}, "difficulty": {"easy"}, "icon": {"drop"}, "area": {c.area()},
		"measure": {measure}, "target": {target}, "step": {step}, "unit": {unit}}
	resp, body := c.do("POST", "/habits/new", form, nil)
	if resp.StatusCode != http.StatusSeeOther {
		c.t.Fatalf("create habit: %s %s", resp.Status, body)
	}
	habits, err := c.q().Habits()
	if err != nil {
		c.t.Fatal(err)
	}
	return habits[len(habits)-1].ID
}

func (c *client) xp() int {
	c.t.Helper()
	bal, err := c.q().Balances(c.svc.Today())
	if err != nil {
		c.t.Fatal(err)
	}
	return bal.XP
}

func TestCounterCard(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.habit("Lesen", "build") // takes the first-check-in badge out of the way below
	id := c.meterHabit("Wasser", "count", "2", "1", "Gläser")
	add := fmt.Sprintf("/habits/%d/add", id)
	undo := fmt.Sprintf("/habits/%d/undo", id)

	_, html := c.get("/")
	if !strings.Contains(html, "0/2") || !strings.Contains(html, ">&#43;1<") || strings.Contains(html, "meter-undo") {
		t.Fatalf("fresh card: %s", html)
	}
	if resp, _ := c.post(undo, true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("undo without entry: %s", resp.Status)
	}
	_, html = c.post(add, true)
	if strings.Contains(html, "<html") || !strings.Contains(html, "1/2") || !strings.Contains(html, "meter-undo") {
		t.Fatalf("after click: %s", html)
	}
	if c.xp() != 0 {
		t.Fatalf("xp before goal: %d", c.xp())
	}
	c.post(add, true)
	earned := c.xp()
	if earned == 0 {
		t.Fatal("no xp at goal")
	}
	c.post(add, true) // over the goal: no second payout
	if c.xp() != earned {
		t.Fatalf("paid twice: %d", c.xp())
	}
	c.post(undo, true)
	if c.xp() != earned {
		t.Fatalf("still at goal: %d", c.xp())
	}
	c.post(undo, true)
	if c.xp() != 0 {
		t.Fatalf("below goal keeps xp: %d", c.xp())
	}
}

func TestQuantityCardAndValidation(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Schritte", "amount", "10000", "1000", "Schritte")
	_, html := c.post(fmt.Sprintf("/habits/%d/add", id), true)
	if !strings.Contains(html, "&#43;1.000") || !strings.Contains(html, "<b>1.000</b>") {
		t.Fatalf("card: %s", html)
	}
	id = c.meterHabit("Lesen", "amount", "20", "5", "Min.")
	_, html = c.post(fmt.Sprintf("/habits/%d/add", id), true)
	if !strings.Contains(html, "&#43;5′") || !strings.Contains(html, "5′") {
		t.Fatalf("minutes: %s", html)
	}
	form := url.Values{"csrf": {c.csrf}, "name": {"Falsch"}, "kind": {"build"}, "freq": {"daily"}, "icon": {"drop"}, "area": {c.area()},
		"measure": {"count"}, "target": {"2"}, "step": {"5"}}
	if resp, _ := c.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("step above goal: %s", resp.Status)
	}
}

func TestFoldHabit(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", "count", "8", "1", "Gläser")
	if _, html := c.get("/"); strings.Contains(html, "hcard collapsed") || !strings.Contains(html, "Wasser einklappen") {
		t.Fatalf("starts open: %s", html)
	}
	folded := map[string]string{"Cookie": fmt.Sprintf("habit_folded=9999.%d", id)}
	_, html := c.do("GET", "/", nil, folded)
	if !strings.Contains(html, "hcard collapsed") || !strings.Contains(html, "Wasser aufklappen") {
		t.Fatalf("folded on this device: %s", html)
	}
	headers := map[string]string{"Cookie": fmt.Sprintf("habit_folded=%d", id), "HX-Request": "true", "X-CSRF-Token": c.csrf}
	if _, html = c.do("POST", fmt.Sprintf("/habits/%d/add", id), url.Values{"csrf": {c.csrf}}, headers); !strings.Contains(html, "1/8") || !strings.Contains(html, "hcard collapsed") {
		t.Fatalf("a folded counter keeps its value: %s", html)
	}
	if _, html = c.get("/"); strings.Contains(html, "hcard collapsed") {
		t.Fatal("another device still sees the open card")
	}
	if _, html = c.do("GET", "/", nil, map[string]string{"Cookie": "habit_folded=x"}); strings.Contains(html, "hcard collapsed") {
		t.Fatalf("a broken cookie folds nothing: %s", html)
	}
}

func TestReachedLine(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	done := c.meterHabit("Wasser", "count", "1", "1", "Gläser")
	second := c.meterHabit("Tee", "count", "1", "1", "Tassen")
	c.meterHabit("Saft", "count", "5", "1", "Gläser")
	c.post(fmt.Sprintf("/habits/%d/add", done), true)
	c.post(fmt.Sprintf("/habits/%d/add", second), true)
	today := c.svc.Today().String()

	_, html := c.get("/")
	for _, want := range []string{`habit-group shut`, `<b>2/3</b>`, `+20 XP`, `+6`, `<b>1</b><small>T</small>`, `2 von 3 erreicht, Liste aufklappen`, "Wasser aufklappen"} {
		if !strings.Contains(html, want) {
			t.Fatalf("starts shut, missing %q in %s", want, html)
		}
	}
	open := func(value string) string {
		_, html := c.do("GET", "/", nil, map[string]string{"Cookie": "reached_open=" + value})
		return html
	}
	if html := open(today); strings.Contains(html, "habit-group shut") || !strings.Contains(html, "Liste einklappen") || strings.Count(html, "Alle einklappen") != 2 {
		t.Fatalf("opens on this device: %s", html)
	}
	if html := open("2020-01-01"); !strings.Contains(html, "habit-group shut") {
		t.Fatalf("an open list only lasts the day: %s", html)
	}
}

func TestLevelFold(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", "count", "8", "1", "Gläser")
	if _, html := c.get("/"); strings.Contains(html, "level-card folded") || !strings.Contains(html, "Level-Anzeige einklappen") {
		t.Fatalf("starts open: %s", html)
	}
	headers := map[string]string{"Cookie": "level_folded=1"}
	_, html := c.do("GET", "/", nil, headers)
	if !strings.Contains(html, "level-card folded") || !strings.Contains(html, "Level-Anzeige aufklappen") || !strings.Contains(html, "XP bis Lv 2") {
		t.Fatalf("folded on this device: %s", html)
	}
	headers = map[string]string{"Cookie": "level_folded=1", "HX-Request": "true", "X-CSRF-Token": c.csrf}
	if _, html = c.do("POST", fmt.Sprintf("/habits/%d/add", id), url.Values{"csrf": {c.csrf}}, headers); !strings.Contains(html, "level-card folded") {
		t.Fatalf("stays folded after a check-in: %s", html)
	}
	if _, html = c.do("GET", "/", nil, map[string]string{"Cookie": "level_folded=0"}); strings.Contains(html, "level-card folded") {
		t.Fatalf("opened again: %s", html)
	}
}

func TestTodayLive(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", "count", "8", "1", "Gläser")
	_, html := c.get("/today/live")
	if !strings.Contains(html, "today-head") || !strings.Contains(html, "0/8") || strings.Contains(html, "<html") {
		t.Fatalf("content of Today without the page around it: %s", html)
	}
	c.post(fmt.Sprintf("/habits/%d/add", id), false)
	if _, html = c.get("/today/live"); !strings.Contains(html, "1/8") {
		t.Fatalf("shows a change from another device: %s", html)
	}
	if _, html = c.do("GET", "/today/live", nil, map[string]string{"Cookie": "level_folded=1"}); !strings.Contains(html, "level-card folded") {
		t.Fatalf("keeps the folds of this device: %s", html)
	}
	if _, html = c.do("GET", "/", nil, nil); !strings.Contains(html, `id="page"`) {
		t.Fatalf("the full page stays: %s", html)
	}
}

func TestGoalZoom(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.reward(model.Reward{Title: "Fernweh", Price: 5000})
	if _, html := c.get("/rewards"); !strings.Contains(html, "--goal-zoom:1") || !strings.Contains(html, "Sparziele kleiner") {
		t.Fatalf("default size: %s", html)
	}
	_, html := c.do("GET", "/rewards", nil, map[string]string{"Cookie": "goal_zoom=115"})
	if !strings.Contains(html, "--goal-zoom:1.15") || !strings.Contains(html, "<b>115%</b>") {
		t.Fatalf("stored size: %s", html)
	}
}

func TestHabitZoom(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.habit("Lesen", "build")
	if _, html := c.get("/"); !strings.Contains(html, "--habit-zoom:1") || !strings.Contains(html, "<b>100%</b>") {
		t.Fatalf("default size: %s", html)
	}
	size := func(value string) string {
		_, html := c.do("GET", "/", nil, map[string]string{"Cookie": "habit_zoom=" + value})
		return html
	}
	if html := size("90"); !strings.Contains(html, "--habit-zoom:0.9") || !strings.Contains(html, "<b>90%</b>") {
		t.Fatalf("stored size: %s", html)
	}
	if html := size("80"); !strings.Contains(html, `aria-label="Habits kleiner" disabled`) {
		t.Fatalf("smallest size: %s", html)
	}
	for _, bad := range []string{"500", "87", "x"} {
		if html := size(bad); !strings.Contains(html, "<b>100%</b>") {
			t.Fatalf("size %s is not on offer: %s", bad, html)
		}
	}
}

func TestFigureGrowsWithTheDay(t *testing.T) {
	for percent, want := range map[int]int{0: 1, 13: 1, 15: 2, 50: 4, 86: 7, 99: 7, 100: 8, 140: 8} {
		if got := figureStage(percent); got != want {
			t.Fatalf("%d %%: stage %d, want %d", percent, got, want)
		}
	}
	c := newApp(t, Config{})
	c.signIn()
	done := c.meterHabit("Lesen", "count", "2", "1", "Seiten")
	c.meterHabit("Wasser", "count", "2", "1", "Gläser")
	if _, html := c.get("/"); !strings.Contains(html, `/static/figures/pflanze-1.svg`) || !strings.Contains(html, "Pflanze, Stufe 1 von 8") {
		t.Fatalf("start of the day: %s", html)
	}
	c.post(fmt.Sprintf("/habits/%d/add", done), true)
	if _, html := c.get("/"); !strings.Contains(html, `/static/figures/pflanze-2.svg`) {
		t.Fatalf("a click counts: %s", html)
	}
	save := func(key string) *http.Response {
		resp, _ := c.do("POST", "/settings/figure", url.Values{"csrf": {c.csrf}, "figure": {key}}, nil)
		return resp
	}
	if _, html := c.get("/settings/figure"); !strings.Contains(html, "Roboter-Bau") || !strings.Contains(html, "figures/platine-8.svg") {
		t.Fatalf("choice: %s", html)
	}
	if resp := save("roboter"); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s", resp.Status)
	}
	if _, html := c.get("/"); !strings.Contains(html, "figures/roboter-2.svg") {
		t.Fatalf("chosen figure: %s", html)
	}
	if resp := save("drache"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown figure: %s", resp.Status)
	}
	save(noFigure)
	if _, html := c.get("/"); strings.Contains(html, "level-figure") {
		t.Fatalf("figure is off: %s", html)
	}
	if _, html := c.get("/settings/figure"); !strings.Contains(html, `value="keine" checked`) {
		t.Fatalf("off in the choice: %s", html)
	}
	for _, group := range figureGroups {
		for _, fig := range group.Figures {
			for stage := 1; stage <= figureStages; stage++ {
				if resp, _ := c.get(fmt.Sprintf("/static/figures/%s-%d.svg", fig.Key, stage)); resp.StatusCode != http.StatusOK {
					t.Fatalf("%s %d: %s", fig.Key, stage, resp.Status)
				}
			}
		}
	}
}

func TestGoalBars(t *testing.T) {
	today := model.DayOf(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC), time.UTC)
	count := func(bars []string, class string) int {
		n := 0
		for _, b := range bars {
			if b == class {
				n++
			}
		}
		return n
	}
	h := game.NewHabitData(model.Habit{Measure: model.MeasureAmount, Frequency: model.FreqDaily, Target: 3, Step: 0.5}, today-5)
	h.Entries[today] = 1.5
	if b, gold := goalBars(goalUnits(h, today, 0, false)); len(b) != 6 || count(b, "on") != 3 || gold != 0 {
		t.Fatalf("3 l in steps of 0,5: %v", b)
	}
	h.Entries[today] = 3.5
	if b, gold := goalBars(goalUnits(h, today, 0, true)); len(b) != 7 || count(b, "on") != 6 || count(b, "gold") != 1 || gold != 16 {
		t.Fatalf("one step beyond the goal: %v %d", b, gold)
	}
	tenths := game.NewHabitData(model.Habit{Measure: model.MeasureAmount, Frequency: model.FreqDaily, Target: 0.6, Step: 0.2}, today-5)
	if b, _ := goalBars(goalUnits(tenths, today, 0, false)); len(b) != 3 {
		t.Fatalf("decimal noise: %v", b)
	}
	many := game.NewHabitData(model.Habit{Measure: model.MeasureCount, Frequency: model.FreqDaily, Target: 1000, Step: 1}, today-5)
	many.Entries[today] = 500
	if b, _ := goalBars(goalUnits(many, today, 0, false)); len(b) != maxBars || count(b, "on") != maxBars/2 {
		t.Fatalf("many steps: %v", b)
	}
	if b, gold := goalBars(20, 40); len(b) != maxBars || count(b, "on") != 15 || count(b, "gold") != 15 || gold != 100 {
		t.Fatalf("twice the goal in many steps: %v %d", b, gold)
	}
	if b, _ := goalBars(1000, 1001); len(b) != maxBars || count(b, "gold") != 1 {
		t.Fatalf("a small excess stays visible: %v", b)
	}
	week := game.NewHabitData(model.Habit{Frequency: model.FreqWeekly, WeeklyTarget: 3}, today-5)
	if b, _ := goalBars(goalUnits(week, today, 2, false)); len(b) != 3 || count(b, "on") != 2 {
		t.Fatalf("weekly rhythm: %v", b)
	}
	if b, _ := goalBars(goalUnits(week, today, 4, false)); len(b) != 4 || count(b, "gold") != 1 {
		t.Fatalf("a fourth check-in: %v", b)
	}
	daily := game.NewHabitData(model.Habit{Frequency: model.FreqDaily}, today-5)
	if b, _ := goalBars(goalUnits(daily, today, 0, true)); len(b) != 1 || count(b, "on") != 1 {
		t.Fatalf("plain habit: %v", b)
	}
}

func TestOverLabel(t *testing.T) {
	habit := func(measure, unit string) *game.HabitData {
		return game.NewHabitData(model.Habit{Measure: measure, Unit: unit}, 0)
	}
	for _, c := range []struct {
		h           *game.HabitData
		value, goal float64
		want        string
	}{
		{habit(model.MeasureAmount, "Liter"), 3.5, 3, "3,5/3 l"},
		{habit(model.MeasureAmount, "Min."), 40, 20, "40/20 min"},
		{habit(model.MeasureAmount, "Schritte"), 12500, 10000, "12,5k/10k Schr."},
		{habit(model.MeasureAmount, "km"), 6.5, 5, "6,5/5 km"},
		{habit(model.MeasureCount, "Gläser"), 9, 8, "9/8 Gläser"},
		{habit("", ""), 4, 3, "4/3"},
	} {
		if got := overLabel(c.h, c.value, c.goal); got != c.want {
			t.Fatalf("%s: got %s", c.want, got)
		}
	}
}

func TestGoalPassedShowsGold(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Lesen", "amount", "20", "20", "Min.")
	add := fmt.Sprintf("/habits/%d/add", id)
	c.post(add, true)
	if _, html := c.get("/"); strings.Contains(html, "ring-gold") || strings.Contains(html, "week-count over") {
		t.Fatalf("exactly the goal stays green: %s", html)
	}
	c.post(add, true)
	_, html := c.get("/")
	for _, want := range []string{"ring-gold", `<b>1</b>/7</span><span class="week-count over">40/20 min</span>`, `<i class="gold"></i>`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
}

func TestTodayGroups(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.meterHabit("Wasser", "amount", "3", "0.5", "l")
	done := c.meterHabit("Lesen", "amount", "10", "5", "Min.")
	c.post(fmt.Sprintf("/habits/%d/add", done), true)
	c.post(fmt.Sprintf("/habits/%d/add", done), true)
	_, html := c.get("/")
	for _, want := range []string{"Offen <em>1</em>", "Erreicht <em>1</em>", "Alle einklappen", "Alle aufklappen", "hcard collapsed reached", "meter-undo", "meter-add"} {
		if !strings.Contains(html, want) {
			t.Fatalf("today: missing %q in %s", want, html)
		}
	}
	if n := strings.Count(html, `<span class="hbars"`); n != 2 {
		t.Fatalf("bars on %d cards", n)
	}
	c.habit("Kein Zucker", "avoid")
	c.habit("Kein Alkohol", "avoid")
	if _, html = c.get("/"); !strings.Contains(html, "Vermeiden <em>2</em>") || strings.Count(html, "data-fold-all") != 3 {
		t.Fatalf("avoid group: %s", html)
	}
}

func TestReachedHabitOpens(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", "count", "1", "1", "Gläser")
	c.post(fmt.Sprintf("/habits/%d/add", id), true)
	for {
		pending, _, _, err := c.q().NextCelebration()
		if err != nil || pending == 0 {
			break
		}
		if err := c.q().DeleteCelebration(pending); err != nil {
			t.Fatal(err)
		}
	}
	today := c.svc.Today()
	if _, html := c.get("/"); !strings.Contains(html, "hcard collapsed") || !strings.Contains(html, fmt.Sprintf(`data-fold="%d" data-day="%s"`, id, today)) {
		t.Fatalf("a reached habit folds and can open: %s", html)
	}
	opened := func(day model.Day) string {
		_, html := c.do("GET", "/", nil, map[string]string{"Cookie": fmt.Sprintf("habit_opened=%s:%d", day, id)})
		return html
	}
	if html := opened(today); strings.Contains(html, "hcard collapsed") || !strings.Contains(html, "Wasser einklappen") {
		t.Fatalf("opened on this device: %s", html)
	}
	if html := opened(today - 1); !strings.Contains(html, "hcard collapsed") {
		t.Fatal("a new day folds the reached habit again")
	}
}

func TestWeeklyGoalFoldsForTheWeek(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	if _, html := c.get("/habits/new?custom=1"); !strings.Contains(html, "1× / Woche") {
		t.Fatal("once a week is not on offer")
	}
	today := c.svc.Today()
	if today.WeekdayIndex() == 0 {
		t.Skip("needs an earlier day in the same week")
	}
	form := url.Values{"csrf": {c.csrf}, "name": {"Idee"}, "kind": {"build"}, "freq": {"w1"}, "difficulty": {"easy"}, "icon": {"book"}, "area": {c.area()}}
	if resp, body := c.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create: %s %s", resp.Status, body)
	}
	habits, err := c.q().Habits()
	if err != nil {
		t.Fatal(err)
	}
	h := habits[0]
	if _, html := c.get("/"); !strings.Contains(html, "Idee") || strings.Contains(html, "hcard collapsed") {
		t.Fatal("an open goal shows the full card")
	}
	c.post(fmt.Sprintf("/habits/%d/check?date=%s", h.ID, (today-1).String()), true)
	if _, html := c.get("/"); !strings.Contains(html, "hcard collapsed") || strings.Contains(html, "Idee einklappen") {
		t.Fatalf("a met weekly goal folds: %s", html)
	}
}

func TestWeeklyAmountGoal(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	form := url.Values{"csrf": {c.csrf}, "name": {"Idee"}, "kind": {"build"}, "freq": {"daily"}, "difficulty": {"easy"}, "icon": {"book"}, "area": {c.area()},
		"measure": {"count"}, "goal_period": {"week"}, "target": {"2"}, "step": {"1"}, "unit": {"Ideen"}}
	if resp, body := c.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create: %s %s", resp.Status, body)
	}
	habits, err := c.q().Habits()
	if err != nil {
		t.Fatal(err)
	}
	h := habits[0]
	if !h.GoalWeek || h.Frequency != model.FreqWeekly || h.WeeklyTarget != 1 {
		t.Fatalf("stored: %+v", h)
	}
	add := fmt.Sprintf("/habits/%d/add", h.ID)
	undo := fmt.Sprintf("/habits/%d/undo", h.ID)
	xp := c.xp()
	if _, html := c.post(add, true); !strings.Contains(html, "<b>1/2</b>") || strings.Contains(html, "hcard collapsed") {
		t.Fatalf("half way: %s", html)
	}
	if c.xp() != xp {
		t.Fatal("no xp before the goal of the week")
	}
	c.post(add, true)
	for {
		if resp, _ := c.get("/celebrate"); resp.StatusCode == http.StatusSeeOther {
			break
		}
		c.post("/celebrate", false)
	}
	reached := c.xp()
	if reached <= xp {
		t.Fatal("reaching the goal pays")
	}
	if _, html := c.get("/"); !strings.Contains(html, "hcard collapsed") {
		t.Fatal("a reached weekly goal folds")
	}
	c.post(add, true)
	if c.xp() != reached {
		t.Fatal("a click past the goal pays nothing")
	}
	checkins := func() int {
		w, err := c.svc.World(c.q())
		if err != nil {
			t.Fatal(err)
		}
		return len(service.Find(w, h.ID).Done)
	}
	if checkins() != 1 {
		t.Fatalf("one check-in per week: %d", checkins())
	}
	c.post(undo, true)
	c.post(undo, true)
	if checkins() != 0 || c.xp() != xp {
		t.Fatalf("falling below takes it back: %d, xp %d", checkins(), c.xp())
	}
	_, html := c.get(fmt.Sprintf("/habits/%d/edit", h.ID))
	if !strings.Contains(html, ">Wochenziel<") {
		t.Fatal("the form names the weekly goal")
	}
	_, html = c.get(fmt.Sprintf("/habits/%d?view=week", h.ID))
	if !strings.Contains(html, "2 Ideen pro Woche") || !strings.Contains(html, "Ziel 2 Ideen") || strings.Contains(html, "goal-line") {
		t.Fatalf("detail: %s", html)
	}
}

func TestHabitOffTodayStaysVisible(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	tomorrow := (c.svc.Today() + 1).WeekdayIndex()
	form := url.Values{"csrf": {c.csrf}, "name": {"Bewerbung"}, "kind": {"build"}, "freq": {"custom"}, "days": {strconv.Itoa(tomorrow)},
		"difficulty": {"easy"}, "icon": {"book"}, "area": {c.area()}}
	if resp, body := c.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create: %s %s", resp.Status, body)
	}
	_, html := c.get("/")
	for _, want := range []string{"Heute frei", "Bewerbung", "wieder am " + weekNames[tomorrow], "hcard collapsed"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q: %s", want, html)
		}
	}
}

func TestCalendarFillsMeasuredDay(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", "count", "8", "1", "Gläser")
	c.post(fmt.Sprintf("/habits/%d/check", id), false)
	_, html := c.get("/")
	if !strings.Contains(html, "hcard collapsed") {
		t.Fatalf("check fills the goal: %s", html)
	}
	c.post(fmt.Sprintf("/habits/%d/check", id), false)
	_, html = c.get("/")
	if !strings.Contains(html, "0/8") {
		t.Fatalf("second check clears the day: %s", html)
	}
}

func TestDeleteHabit(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")
	c.post(fmt.Sprintf("/habits/%d/check", id), false)
	earned := c.xp()
	if err := c.q().SaveReward(model.Reward{Title: "Buch", Mode: model.RewardStreak, HabitID: id, StreakTarget: 3}); err != nil {
		t.Fatal(err)
	}
	if resp, _ := c.post(fmt.Sprintf("/habits/%d/delete", id), false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete: %s", resp.Status)
	}
	if resp, _ := c.get(fmt.Sprintf("/habits/%d", id)); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("still there: %s", resp.Status)
	}
	if c.xp() < earned {
		t.Fatalf("earned xp lost: %d of %d", c.xp(), earned)
	}
	rewards, err := c.q().Rewards()
	if err != nil {
		t.Fatal(err)
	}
	if len(rewards) != 1 || !rewards[0].Archived {
		t.Fatalf("streak reward: %+v", rewards)
	}
	if next := c.habit("Neu", "build"); next == id {
		t.Fatal("id reused, the new habit would inherit old ledger entries")
	}
}

func TestDeleteAllHabits(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	first := c.habit("Lesen", "build")
	c.habit("Sport", "build")
	c.post(fmt.Sprintf("/habits/%d/check", first), false)
	c.post(fmt.Sprintf("/habits/%d/archive", first), false)
	earned := c.xp()
	_, html := c.get("/habits")
	if !strings.Contains(html, `action="/habits/delete"`) || !strings.Contains(html, "Lesen löschen") || !strings.Contains(html, "Sport löschen") {
		t.Fatalf("delete controls: %s", html)
	}
	if resp, _ := c.post("/habits/delete", false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete all: %s", resp.Status)
	}
	habits, err := c.q().Habits()
	if err != nil || len(habits) != 0 {
		t.Fatalf("left: %v %v", habits, err)
	}
	if c.xp() < earned {
		t.Fatalf("earned xp lost: %d of %d", c.xp(), earned)
	}
	if resp, _ := c.get("/"); resp.StatusCode >= http.StatusBadRequest {
		t.Fatalf("today without habits: %s", resp.Status)
	}
	_, html = c.get("/habits")
	if strings.Contains(html, `action="/habits/delete"`) {
		t.Fatal("button shown without habits")
	}
}

func TestResetStartsOver(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")
	c.post(fmt.Sprintf("/habits/%d/check", id), false)
	q := c.q()
	if err := q.SaveReward(model.Reward{Title: "Kino", Mode: model.RewardCoins, Price: 10}); err != nil {
		t.Fatal(err)
	}
	if err := q.SaveReward(model.Reward{Title: "Buch", Mode: model.RewardStreak, HabitID: id, StreakTarget: 3}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetSetting(service.KeyLevel, "12"); err != nil {
		t.Fatal(err)
	}
	if err := q.SetSetting(service.KeyInstalledOn, "2020-01-01"); err != nil {
		t.Fatal(err)
	}
	areas, _ := q.Areas()
	if c.xp() == 0 {
		t.Fatal("nothing earned before the reset")
	}

	if resp, _ := c.post("/settings/reset", false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("reset: %s", resp.Status)
	}
	bal, err := q.Balances(c.svc.Today())
	if err != nil {
		t.Fatal(err)
	}
	if bal.XP != 0 || bal.Coins != 0 || bal.Jokers != 0 || bal.CoinsEarned != 0 {
		t.Fatalf("balances: %+v", bal)
	}
	if habits, _ := q.Habits(); len(habits) != 0 {
		t.Fatalf("habits left: %v", habits)
	}
	if unlocked, _ := q.UnlockedBadges(); len(unlocked) != 0 {
		t.Fatalf("badges left: %v", unlocked)
	}
	if level, _ := q.SettingInt(service.KeyLevel, 0); level != 1 {
		t.Fatalf("level %d", level)
	}
	if left, _ := q.Areas(); len(left) != len(areas) {
		t.Fatalf("areas: %d of %d", len(left), len(areas))
	}
	if rewards, _ := q.Rewards(); len(rewards) != 0 {
		t.Fatalf("rewards: %+v", rewards)
	}
	if resp, _ := c.get("/"); resp.Header.Get("Location") != "/onboarding" {
		t.Fatalf("next: %s %q", resp.Status, resp.Header.Get("Location"))
	}
	if _, html := c.get("/settings/server"); !strings.Contains(html, "Abmelden") {
		t.Fatal("still signed in expected")
	}
	if _, html := c.get("/profile"); !strings.Contains(html, `action="/logout"`) || !strings.Contains(html, "Level 1 ") {
		t.Fatalf("profile: %s", html)
	}
}

func TestWeekStrip(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", "count", "4", "1", "Gläser")
	_, html := c.get("/")
	if !strings.Contains(html, "KW ") || !strings.Contains(html, "erreicht") || !strings.Contains(html, "wd open today") {
		t.Fatalf("empty week: %s", html)
	}
	_, html = c.post(fmt.Sprintf("/habits/%d/add", id), true)
	if !strings.Contains(html, "wd part today") || !strings.Contains(html, "height:25%") {
		t.Fatalf("partial day: %s", html)
	}
	for range 3 {
		c.post(fmt.Sprintf("/habits/%d/add", id), true)
	}
	_, html = c.get("/")
	if !strings.Contains(html, "wd done today") || !strings.Contains(html, `Diese Woche: 1 von 7 erreicht`) {
		t.Fatalf("reached day: %s", html)
	}
}

func TestMonthCountsAFinishedToday(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Lesen", "amount", "5", "5", "Min.")
	stat := func() string {
		_, html := c.get(fmt.Sprintf("/habits/%d?view=week", id))
		return regexp.MustCompile(`(\d+)/\d+</b><small>Ziel erreicht`).FindStringSubmatch(html)[1]
	}
	if got := stat(); got != "0" {
		t.Fatalf("an open first day is not done: %s", got)
	}
	if _, html := c.get(fmt.Sprintf("/habits/%d", id)); strings.Contains(html, "Muster") {
		t.Fatalf("a fresh habit has no patterns yet: %s", html)
	}
	c.post(fmt.Sprintf("/habits/%d/add", id), true)
	if got := stat(); got != "1" {
		t.Fatalf("finished today: %s", got)
	}
	avoid := c.habit("Kein Zucker", "avoid")
	c.post(fmt.Sprintf("/habits/%d/relapse", avoid), true)
	_, html := c.get(fmt.Sprintf("/habits/%d?view=week", avoid))
	if !strings.Contains(html, "0/1</b>") {
		t.Fatal("a relapse today counts as a missed day")
	}
}

func TestDetailRemembersTheViewPerHabit(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	one := c.habit("Eins", "build")
	two := c.habit("Zwei", "build")
	on := func(id int64, query string) string {
		_, html := c.get(fmt.Sprintf("/habits/%d%s", id, query))
		return regexp.MustCompile(`class="on">(Woche|Monat|Jahr)<`).FindStringSubmatch(html)[1]
	}
	if got := on(one, ""); got != "Monat" {
		t.Fatalf("default: %s", got)
	}
	if got := on(one, "?view=year"); got != "Jahr" {
		t.Fatalf("chosen: %s", got)
	}
	on(two, "?view=week")
	if got := on(one, ""); got != "Jahr" {
		t.Fatalf("first habit: %s", got)
	}
	if got := on(two, ""); got != "Woche" {
		t.Fatalf("second habit: %s", got)
	}
	// A check-in in the calendar keeps it as well.
	c.do("POST", fmt.Sprintf("/habits/%d/check?from=detail&view=week", two), url.Values{}, map[string]string{"X-CSRF-Token": c.csrf, "HX-Request": "true"})
	if got := on(two, ""); got != "Woche" {
		t.Fatalf("after a check-in: %s", got)
	}
	dump, err := c.q().DumpAll()
	if err != nil {
		t.Fatal(err)
	}
	for key := range dump.Settings {
		if strings.HasPrefix(key, "habit_view") {
			t.Fatalf("view in the export: %s", key)
		}
	}
}

func TestDetailChartsFollowTheEntries(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Lesen", "amount", "20", "5", "Min.")
	add := fmt.Sprintf("/habits/%d/add", id)
	c.post(add, true)
	_, html := c.get(fmt.Sprintf("/habits/%d?view=week", id))
	for _, want := range []string{"goal-line", "Minuten pro Tag", "<b class=\"\">5′</b>", "5 Min.", "Ø pro Tag", "Ziel erreicht", "läuft"} {
		if !strings.Contains(html, want) {
			t.Fatalf("week: missing %q", want)
		}
	}
	_, html = c.get(fmt.Sprintf("/habits/%d?view=month", id))
	if !strings.Contains(html, "--fill:25%") || !strings.Contains(html, "day part today") {
		t.Fatal("month: a partly reached day needs its fill")
	}
	for range 3 {
		c.post(add, true)
	}
	_, html = c.get(fmt.Sprintf("/habits/%d?view=year", id))
	for _, want := range []string{"<b>20</b> <span>Minuten</span>", "<b>100%</b>", "Stunden pro Monat", "bester Monat"} {
		if !strings.Contains(html, want) {
			t.Fatalf("year: missing %q", want)
		}
	}
	check := c.habit("Abhaken", "build")
	_, html = c.get(fmt.Sprintf("/habits/%d?view=week", check))
	if !strings.Contains(html, "0 von 1 Tagen") || strings.Contains(html, "goal-line") {
		t.Fatal("a tick habit has no goal line")
	}
}

func TestStepsBelowOne(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", "amount", "3", "0,1", "L")
	add := fmt.Sprintf("/habits/%d/add", id)
	h, _ := c.q().Habit(id)
	if h.Step != 0.1 || h.Target != 3 {
		t.Fatalf("habit: %+v", h)
	}
	var html string
	for range 3 {
		_, html = c.post(add, true)
	}
	if !strings.Contains(html, "<b>0,3</b>") || !strings.Contains(html, ">&#43;0,1<") {
		t.Fatalf("0,1 × 3 must not show rounding noise: %s", html)
	}
	for range 27 {
		c.post(add, true)
	}
	if c.xp() == 0 {
		t.Fatal("30 × 0,1 must reach the goal of 3")
	}
	form := url.Values{"csrf": {c.csrf}, "name": {"Falsch"}, "kind": {"build"}, "freq": {"daily"}, "icon": {"drop"}, "area": {c.area()}, "measure": {"amount"}, "target": {"1"}, "step": {"0"}}
	if resp, _ := c.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("step 0: %s", resp.Status)
	}
}

func TestResetRemovesRewards(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	if err := c.q().SaveReward(model.Reward{Title: "Kino", Mode: model.RewardCoins, Price: 10}); err != nil {
		t.Fatal(err)
	}
	c.post("/settings/reset", false)
	if rewards, _ := c.q().Rewards(); len(rewards) != 0 {
		t.Fatalf("rewards left: %+v", rewards)
	}
	if _, html := c.get("/rewards"); !strings.Contains(html, "Noch keine Belohnungen") {
		t.Fatal("no empty state")
	}
}

func TestBadgesCanBeSwitchedOff(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	if _, html := c.get("/profile"); !strings.Contains(html, `href="/badges"`) {
		t.Fatal("tab missing")
	}
	if resp, _ := c.do("POST", "/admin/badges/feature", url.Values{"csrf": {c.csrf}}, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("switch: %s", resp.Status)
	}
	if _, html := c.get("/profile"); strings.Contains(html, `href="/badges"`) {
		t.Fatal("tab still shown")
	}
	if resp, _ := c.get("/badges"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("page: %s", resp.Status)
	}
	id := c.habit("Lesen", "build")
	c.post(fmt.Sprintf("/habits/%d/check", id), false)
	if badges, _ := c.q().UnlockedBadges(); len(badges) != 0 {
		t.Fatalf("awarded while off: %v", badges)
	}
	c.do("POST", "/admin/badges/feature", url.Values{"csrf": {c.csrf}, "enabled": {"on"}}, nil)
	if _, html := c.get("/profile"); !strings.Contains(html, `href="/badges"`) {
		t.Fatal("tab not back")
	}
}
