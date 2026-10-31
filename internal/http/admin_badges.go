package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/templates"
)

// ownFilter is the category filter for badges made in the admin area.
const ownFilter = "_eigene"

var badgeColors = []string{"#F2994A", "#4FB283", "#5FD3C6", "#C79BF2", "#7FB8F0", "#F28BB0", "#F2C14E", "#9FB4C8", "#8C8CF5", "#A3A69F"}

type badgeRow struct {
	game.Badge
	CategoryName string
	Accent       string
	Condition    string
}

type categoryFilter struct {
	Key   string
	Name  string
	Color string
	Count int
}

type badgesAdminData struct {
	Rows     []badgeRow
	Filters  []categoryFilter
	Own      int
	Builtin  int
	Disabled int
	Total    int
	Category string
	Query    string
	Status   string
	Statuses []badgeFilter
}

func unitOf(rule string) string {
	switch rule {
	case game.RuleStreak:
		return "Tage"
	case game.RuleCount:
		return "Check-ins"
	}
	return "Einheiten"
}

func areaNames(areas []model.Area) func(string) string {
	return func(key string) string {
		for _, a := range areas {
			if a.Key == key {
				return a.Name
			}
		}
		return key
	}
}

func (s *Server) defaultAreas(r *http.Request) ([]model.Area, error) {
	return s.svc.Store.Q(r.Context()).DefaultAreas()
}

func matchesBadge(b game.Badge, category, query, status string) bool {
	switch {
	case category == ownFilter && b.Builtin:
		return false
	case category != "" && category != ownFilter && b.Category != category:
		return false
	case status == "aktiv" && !b.Active, status == "deaktiviert" && b.Active, status == "geheim" && !b.Secret:
		return false
	}
	if query == "" {
		return true
	}
	return strings.Contains(templates.Fold(b.Name+" "+b.Hint+" "+b.Description), templates.Fold(query))
}

func (s *Server) adminBadges(w http.ResponseWriter, r *http.Request) error {
	areas, err := s.defaultAreas(r)
	if err != nil {
		return err
	}
	rules := s.svc.Rules()
	d := badgesAdminData{Category: r.FormValue("cat"), Query: strings.TrimSpace(r.FormValue("q")), Status: r.FormValue("status"), Total: len(rules.Badges),
		Statuses: []badgeFilter{{"", "Alle"}, {"aktiv", "Aktiv"}, {"deaktiviert", "Deaktiviert"}, {"geheim", "Geheim"}}}
	counts := map[string]int{}
	names := areaNames(areas)
	for _, b := range rules.Badges {
		counts[b.Category]++
		switch {
		case b.Builtin:
			d.Builtin++
		default:
			d.Own++
		}
		if !b.Active {
			d.Disabled++
		}
		if !matchesBadge(b, d.Category, d.Query, d.Status) {
			continue
		}
		c, _ := rules.CategoryByKey(b.Category)
		d.Rows = append(d.Rows, badgeRow{Badge: b, CategoryName: c.Name, Accent: badgeColor(b, c), Condition: b.Condition(names)})
	}
	for _, c := range rules.Categories {
		d.Filters = append(d.Filters, categoryFilter{Key: c.Key, Name: c.Name, Color: c.Color, Count: counts[c.Key]})
	}
	p := s.adminPage(r, "Abzeichen", "badges", d)
	if isHTMX(r) {
		return s.view.Fragment(w, "admin_badges", "badge-rows", p)
	}
	return s.view.Render(w, http.StatusOK, "admin_badges", p)
}

type badgeFormData struct {
	Badge      game.Badge
	New        bool
	Categories []game.Category
	Areas      []model.Area
	Icons      []string
	Colors     []string
	Holders    int
	Config     string
	Error      string
	Special    bool
	Rule       string
	Scope      string
	Value      string
	Jokers     bool
}

// badgeIcons offers every icon a badge or a habit uses.
func badgeIcons(rules *game.Rules) []string {
	var out []string
	add := func(name string) {
		if name != "" && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	for _, b := range rules.Badges {
		add(b.Icon)
	}
	for _, c := range habitIcons {
		add(c.Name)
	}
	return out
}

func (s *Server) badgeForm(w http.ResponseWriter, r *http.Request, b game.Badge, isNew bool, status int, message string) error {
	rules := s.svc.Rules()
	areas, err := s.defaultAreas(r)
	if err != nil {
		return err
	}
	holders := 0
	if !isNew {
		if holders, err = s.svc.Store.Q(r.Context()).BadgeHolders(b.Key); err != nil {
			return err
		}
	}
	m := b.Metric
	d := badgeFormData{Badge: b, New: isNew, Categories: rules.Categories, Areas: areas, Icons: badgeIcons(rules), Colors: badgeColors,
		Holders: holders, Error: message, Special: m.Rule == game.RuleSpecial, Rule: m.Rule, Scope: m.Scope, Value: m.Value, Jokers: m.Jokers}
	catColors := map[string]string{}
	for _, c := range rules.Categories {
		catColors[c.Key] = c.Color
	}
	config, err := json.Marshal(map[string]any{"name": b.Name, "icon": b.Icon, "color": b.Color, "category": b.Category, "catColors": catColors,
		"rule": m.Rule, "scope": m.Scope, "value": m.Value, "target": strconv.Itoa(b.Target), "jokers": m.Jokers, "key": b.Key,
		"sprite": "/static/icons/sprite.svg?v=" + s.view.Version, "fixedHint": b.Hint})
	if err != nil {
		return err
	}
	d.Config = string(config)
	title := "Abzeichen bearbeiten"
	if isNew {
		title = "Neues Abzeichen"
	}
	return s.view.Render(w, status, "admin_badge_form", s.adminPage(r, title, "badges", d))
}

func (s *Server) adminBadgeNew(w http.ResponseWriter, r *http.Request) error {
	rules := s.svc.Rules()
	b := game.Badge{Icon: "flame", Target: 7, Active: true, Coins: game.BadgeCoins, Sort: len(rules.Badges) + 1,
		Metric: game.Metric{Rule: game.RuleStreak, Scope: game.ScopeAnyHabit}}
	if len(rules.Categories) > 0 {
		b.Category = rules.Categories[0].Key
	}
	return s.badgeForm(w, r, b, true, http.StatusOK, "")
}

func (s *Server) adminBadgeEdit(w http.ResponseWriter, r *http.Request) error {
	b, ok := s.svc.Rules().BadgeByKey(r.PathValue("key"))
	if !ok {
		return notFound()
	}
	return s.badgeForm(w, r, b, false, http.StatusOK, "")
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func uniqueKey(name string, taken func(string) bool) string {
	base := strings.Trim(nonSlug.ReplaceAllString(templates.Fold(name), "-"), "-")
	if base == "" {
		base = "abzeichen"
	}
	key := "eigen-" + base
	for i := 2; taken(key); i++ {
		key = fmt.Sprintf("eigen-%s-%d", base, i)
	}
	return key
}

// parseBadge applies the form. Special badges keep their rule: the logic behind
// it lives in the code and has no target to edit in the builder.
func parseBadge(r *http.Request, b *game.Badge, rules *game.Rules, icons []string, areas []model.Area) string {
	b.Name = strings.TrimSpace(r.FormValue("name"))
	b.Description = strings.TrimSpace(r.FormValue("description"))
	b.Category = r.FormValue("category")
	b.Icon = r.FormValue("icon")
	b.Color = r.FormValue("color")
	b.Secret = r.FormValue("secret") == "on"
	var err error
	if b.Coins, err = strconv.Atoi(r.FormValue("coins")); err != nil || b.Coins < 0 {
		return "Die Taler-Belohnung muss 0 oder mehr sein"
	}
	if b.Jokers, err = strconv.Atoi(r.FormValue("jokers")); err != nil || b.Jokers < 0 || b.Jokers > game.MaxJokers {
		return "Die Joker-Belohnung muss zwischen 0 und 3 liegen"
	}
	if b.Sort, err = strconv.Atoi(r.FormValue("sort")); err != nil {
		return "Die Reihenfolge muss eine Zahl sein"
	}
	if _, ok := rules.CategoryByKey(b.Category); !ok {
		return "Bitte wähle eine Kategorie"
	}
	switch {
	case b.Name == "" || len(b.Name) > 60:
		return "Bitte gib einen Namen ein"
	case !slices.Contains(icons, b.Icon):
		return "Bitte wähle ein Icon"
	case b.Color != "" && !colorPattern.MatchString(b.Color):
		return "Ungültige Farbe"
	}
	if b.Metric.Rule == game.RuleSpecial {
		return ""
	}
	return parseRule(r, b, areas)
}

func parseRule(r *http.Request, b *game.Badge, areas []model.Area) string {
	m := game.Metric{Rule: r.FormValue("rule"), Scope: r.FormValue("scope"), Jokers: r.FormValue("count_jokers") == "on"}
	switch m.Rule {
	case game.RuleStreak, game.RuleCount, game.RuleAmount:
	default:
		return "Bitte wähle einen Typ"
	}
	switch m.Scope {
	case game.ScopeAnyHabit, game.ScopeKindBuild, game.ScopeKindAvoid:
	case game.ScopeHabitTag:
		m.Value = strings.ToLower(strings.TrimSpace(r.FormValue("tag")))
		if m.Value == "" {
			return "Bitte gib einen Tag ein"
		}
	case game.ScopeArea:
		m.Value = r.FormValue("area")
		if !slices.ContainsFunc(areas, func(a model.Area) bool { return a.Key == m.Value }) {
			return "Bitte wähle einen Bereich"
		}
	default:
		return "Bitte wähle einen Bezug"
	}
	target, err := strconv.Atoi(r.FormValue("target"))
	if err != nil || target < 1 || target > 100000000 {
		return "Der Zielwert muss mindestens 1 sein"
	}
	if target != b.Target || b.Hint == "" || m.Rule != b.Metric.Rule {
		b.Hint = fmt.Sprintf("%d %s", target, unitOf(m.Rule))
	}
	b.Target, b.Metric = target, m
	return ""
}

func (s *Server) adminBadgeSave(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return badRequest("Ungültiges Formular")
	}
	rules := s.svc.Rules()
	key := r.PathValue("key")
	b := game.Badge{Active: true, Metric: game.Metric{Rule: game.RuleStreak, Scope: game.ScopeAnyHabit}}
	if key != "" {
		var ok bool
		if b, ok = rules.BadgeByKey(key); !ok {
			return notFound()
		}
	}
	areas, err := s.defaultAreas(r)
	if err != nil {
		return err
	}
	if message := parseBadge(r, &b, rules, badgeIcons(rules), areas); message != "" {
		return s.badgeForm(w, r, b, key == "", http.StatusUnprocessableEntity, message)
	}
	if key == "" {
		b.Key = uniqueKey(b.Name, func(k string) bool { _, ok := rules.BadgeByKey(k); return ok })
	}
	if err := s.svc.Store.Q(r.Context()).SaveBadge(b); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/badges")
}

func (s *Server) adminBadgeActive(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.Store.Q(r.Context()).SetBadgeActive(r.PathValue("key"), r.FormValue("active") == "on"); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	if isHTMX(r) {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return redirect(w, r, "/admin/badges")
}

func (s *Server) adminBadgeDelete(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.Store.Tx(r.Context(), func(q *db.Q) error { return q.DeleteBadge(r.PathValue("key")) }); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/badges")
}

type categoriesData struct {
	Rows   []categoryFilter
	Colors []string
	Error  string
}

func (s *Server) adminCategories(w http.ResponseWriter, r *http.Request) error {
	rules := s.svc.Rules()
	counts := map[string]int{}
	for _, b := range rules.Badges {
		counts[b.Category]++
	}
	d := categoriesData{Colors: badgeColors, Error: r.FormValue("error")}
	for _, c := range rules.Categories {
		d.Rows = append(d.Rows, categoryFilter{Key: c.Key, Name: c.Name, Color: c.Color, Count: counts[c.Key]})
	}
	return s.view.Render(w, http.StatusOK, "admin_categories", s.adminPage(r, "Kategorien", "badges", d))
}

func (s *Server) adminCategorySave(w http.ResponseWriter, r *http.Request) error {
	rules := s.svc.Rules()
	c := game.Category{Key: r.FormValue("key"), Name: strings.TrimSpace(r.FormValue("name")), Color: r.FormValue("color")}
	if c.Name == "" || len(c.Name) > 40 || !colorPattern.MatchString(c.Color) {
		return redirect(w, r, "/admin/badges/categories?error=Bitte+Name+und+Farbe+angeben")
	}
	if old, ok := rules.CategoryByKey(c.Key); ok {
		c.Sort = old.Sort
	} else {
		c.Key = strings.TrimPrefix(uniqueKey(c.Name, func(k string) bool {
			_, ok := rules.CategoryByKey(strings.TrimPrefix(k, "eigen-"))
			return ok
		}), "eigen-")
		c.Sort = len(rules.Categories) + 1
	}
	if err := s.svc.Store.Q(r.Context()).SaveCategory(c); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/badges/categories")
}

func (s *Server) adminCategoryDelete(w http.ResponseWriter, r *http.Request) error {
	err := s.svc.Store.Q(r.Context()).DeleteCategory(r.PathValue("key"))
	if errors.Is(err, db.ErrCategoryInUse) {
		return redirect(w, r, "/admin/badges/categories?error=Die+Kategorie+enth%C3%A4lt+noch+Abzeichen")
	}
	if err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/badges/categories")
}

// badgeFile is the JSON form of all badges and categories.
type badgeFile struct {
	Version    int             `json:"version"`
	Categories []categoryEntry `json:"categories"`
	Badges     []badgeEntry    `json:"badges"`
}

type categoryEntry struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Sort  int    `json:"sort"`
}

type badgeEntry struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Hint        string `json:"hint"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	RuleType    string `json:"rule_type"`
	Scope       string `json:"scope"`
	ScopeValue  string `json:"scope_value"`
	Target      int    `json:"target"`
	CountJokers bool   `json:"count_jokers"`
	Secret      bool   `json:"secret"`
	Coins       int    `json:"reward_coins"`
	Jokers      int    `json:"reward_jokers"`
	Sort        int    `json:"sort"`
	Active      bool   `json:"active"`
}

func (s *Server) adminBadgesTransfer(w http.ResponseWriter, r *http.Request) error {
	return s.view.Render(w, http.StatusOK, "admin_badge_transfer", s.adminPage(r, "Import / Export", "badges", r.FormValue("error")))
}

func (s *Server) adminBadgesExport(w http.ResponseWriter, r *http.Request) error {
	rules := s.svc.Rules()
	f := badgeFile{Version: 1}
	for _, c := range rules.Categories {
		f.Categories = append(f.Categories, categoryEntry(c))
	}
	for _, b := range rules.Badges {
		m := b.Metric
		f.Badges = append(f.Badges, badgeEntry{Key: b.Key, Name: b.Name, Hint: b.Hint, Description: b.Description, Category: b.Category, Icon: b.Icon,
			Color: b.Color, RuleType: m.Rule, Scope: m.Scope, ScopeValue: m.Value, Target: b.Target, CountJokers: m.Jokers, Secret: b.Secret,
			Coins: b.Coins, Jokers: b.Jokers, Sort: b.Sort, Active: b.Active})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="habitd-abzeichen.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(f)
}

const transferError = "/admin/badges/transfer?error="

// importBadges merges a file into the rules. Built-in badges stay built in, and
// a special rule only works for the keys that have code behind them.
func importBadges(q *db.Q, f *badgeFile, rules *game.Rules) error {
	for _, c := range f.Categories {
		if c.Key == "" || c.Name == "" || !colorPattern.MatchString(c.Color) {
			return fmt.Errorf("category %q is invalid", c.Key)
		}
		if err := q.SaveCategory(game.Category(c)); err != nil {
			return err
		}
	}
	for _, e := range f.Badges {
		if e.Key == "" || e.Name == "" || e.Target < 1 {
			return fmt.Errorf("badge %q is invalid", e.Key)
		}
		old, known := rules.BadgeByKey(e.Key)
		if e.RuleType == game.RuleSpecial && game.Special(e.Key) == nil {
			return fmt.Errorf("badge %q has no special rule", e.Key)
		}
		b := game.Badge{Key: e.Key, Name: e.Name, Hint: e.Hint, Description: e.Description, Category: e.Category, Icon: e.Icon, Color: e.Color,
			Target: e.Target, Secret: e.Secret, Coins: e.Coins, Jokers: e.Jokers, Sort: e.Sort, Active: e.Active, Builtin: known && old.Builtin,
			Metric: game.Metric{Rule: e.RuleType, Scope: e.Scope, Value: e.ScopeValue, Jokers: e.CountJokers}}
		if err := q.SaveBadge(b); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) adminBadgesImport(w http.ResponseWriter, r *http.Request) error {
	file, _, err := r.FormFile("file")
	if err != nil {
		return redirect(w, r, transferError+"Bitte+w%C3%A4hle+eine+Datei")
	}
	defer file.Close()
	var f badgeFile
	if err := json.NewDecoder(io.LimitReader(file, maxImportSize)).Decode(&f); err != nil || f.Version != 1 {
		return redirect(w, r, transferError+"Datei+nicht+lesbar")
	}
	rules := s.svc.Rules()
	if err := s.svc.Store.Tx(r.Context(), func(q *db.Q) error { return importBadges(q, &f, rules) }); err != nil {
		return redirect(w, r, transferError+"Import+fehlgeschlagen")
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/badges")
}
