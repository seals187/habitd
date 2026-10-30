package server

import (
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
	"strings"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/web"
)

const password = "correct-horse"

type client struct {
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
	if err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	app, err := New(cfg, svc, web.FS)
	if err != nil {
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
	form := url.Values{"csrf": {c.csrf}, "name": {name}, "kind": {kind}, "freq": {"daily"}, "difficulty": {"medium"}, "icon": {"book"}, "area": {"2"}}
	resp, body := c.do("POST", "/habits/new", form, nil)
	if resp.StatusCode != http.StatusSeeOther {
		c.t.Fatalf("create habit: %s %s", resp.Status, body)
	}
	habits, err := c.svc.Store.Q(context.Background()).Habits()
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
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("got %s", resp.Status)
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
	if resp.Header.Get("HX-Redirect") != "/celebrate" {
		t.Fatalf("redirect %q", resp.Header.Get("HX-Redirect"))
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

	// Undo answers with the fragment, not a full document.
	resp, html = c.post(fmt.Sprintf("/habits/%d/check", id), true)
	if resp.StatusCode != 200 || strings.Contains(html, "<html") || !strings.Contains(html, "Lesen") {
		t.Fatalf("fragment: %s %s", resp.Status, html)
	}
	if !strings.Contains(html, `aria-pressed="false"`) {
		t.Fatal("habit should be open after undo")
	}
}

func TestDetailViews(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	if err := c.svc.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{"month": "Tage erledigt", "year": "Bester Wochentag", "week": "Serie"}
	for view, want := range cases {
		for id := 1; id <= 5; id++ {
			resp, html := c.get(fmt.Sprintf("/habits/%d?view=%s", id, view))
			if resp.StatusCode != 200 || !strings.Contains(html, want) {
				t.Fatalf("%s/%d: %s", view, id, resp.Status)
			}
		}
	}
	resp, html := c.get("/habits/1?view=year")
	if resp.StatusCode != 200 || !strings.Contains(html, "212 Tage erledigt") || !strings.Contains(html, "86%") {
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
	if err := c.svc.Seed(context.Background()); err != nil {
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
	ctx := context.Background()
	if err := c.svc.Seed(ctx); err != nil {
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
	if stats["badges_total"] != float64(95) {
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
	habits, err := c.svc.Store.Q(context.Background()).Habits()
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
	habits, _ = c.svc.Store.Q(context.Background()).Habits()
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

func TestLibraryAddAndActive(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	_, html := c.get("/templates?area=umwelt&kind=avoid")
	if !strings.Contains(html, "Plastiktüten") || strings.Contains(html, "Joggen") {
		t.Fatal("filter")
	}
	resp, html := c.post("/templates/plastikt-ten/add", true)
	if resp.StatusCode == 200 {
		t.Fatalf("unknown template accepted: %s", html)
	}
	resp, html = c.post("/templates/joggen/add?q=jog", true)
	if resp.StatusCode != 200 || !strings.Contains(html, "Aktiv") || strings.Contains(html, "<html") {
		t.Fatalf("%s %s", resp.Status, html)
	}
	_, _ = c.post("/templates/joggen/add", true)
	habits, _ := c.svc.Store.Q(context.Background()).Habits()
	if len(habits) != 1 || habits[0].Tag != "sport" {
		t.Fatalf("habits %+v", habits)
	}
}

func TestTemplateTagSurvivesForm(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	form := url.Values{"csrf": {c.csrf}, "name": {"Weniger scrollen"}, "kind": {"avoid"}, "template": {"doomscrolling"}, "icon": {"heart"}, "area": {"8"}, "difficulty": {"easy"}, "freq": {"daily"}}
	if resp, _ := c.do("POST", "/habits/new", form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("got %s", resp.Status)
	}
	habits, _ := c.svc.Store.Q(context.Background()).Habits()
	if habits[0].Tag != "doomscroll" || habits[0].TemplateID != "doomscrolling" {
		t.Fatalf("habit %+v", habits[0])
	}
}
