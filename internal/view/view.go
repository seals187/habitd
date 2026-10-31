// Package view renders the HTML templates.
package view

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// Page is the data every template receives.
type Page struct {
	Title   string
	Tab     string
	CSRF    string
	Version string // cache buster of the static assets
	Data    any
}

// Renderer holds the parsed templates, one set per page.
type Renderer struct {
	pages   map[string]*template.Template
	Version string
}

var weekdays = []string{"Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag", "Sonntag"}

// Months are the German month names, January first.
var Months = []string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}

// New parses all templates from the embedded file system.
func New(files fs.FS) (*Renderer, error) {
	version, err := hashStatic(files)
	if err != nil {
		return nil, err
	}
	funcs := template.FuncMap{
		"icon":       func(name string, size int) template.HTML { return iconHTML(version, name, size) },
		"num":        Number,
		"add":        func(a, b int) int { return a + b },
		"sub":        func(a, b int) int { return a - b },
		"signed":     func(n int) string { return fmt.Sprintf("%+d", n) },
		"dash":       dash,
		"quarter":    func(r float64) string { return fmt.Sprintf("%.2f", 2*math.Pi*r/4) },
		"streak":     StreakLabel,
		"streakUnit": StreakUnit,
		"multiple":   MultiplierLabel,
		"dict":       dict,
	}
	base, err := template.New("base").Funcs(funcs).ParseFS(files, "templates/_*.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	names, err := fs.Glob(files, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	r := &Renderer{pages: map[string]*template.Template{}, Version: version}
	for _, name := range names {
		file := path.Base(name)
		if strings.HasPrefix(file, "_") {
			continue
		}
		t, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone layout: %w", err)
		}
		if _, err := t.ParseFS(files, name); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		r.pages[strings.TrimSuffix(file, ".html")] = t
	}
	return r, nil
}

func hashStatic(files fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(files, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(files, p)
		if err != nil {
			return err
		}
		_, err = h.Write(body)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("hash assets: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

func iconHTML(version, name string, size int) template.HTML {
	return template.HTML(fmt.Sprintf(`<svg class="i" width="%d" height="%d" aria-hidden="true"><use href="/static/icons/sprite.svg?v=%s#%s"/></svg>`,
		size, size, version, template.HTMLEscapeString(name)))
}

// Render writes a full page. Pages render into a buffer first, so a template
// error never ends up as half an HTML document.
func (r *Renderer) Render(w http.ResponseWriter, status int, page string, p Page) error {
	return r.execute(w, status, page, "layout", p)
}

// Fragment writes one named block, for HTMX responses.
func (r *Renderer) Fragment(w http.ResponseWriter, page, block string, p Page) error {
	return r.execute(w, http.StatusOK, page, block, p)
}

func (r *Renderer) execute(w http.ResponseWriter, status int, page, block string, p Page) error {
	t, ok := r.pages[page]
	if !ok {
		return fmt.Errorf("unknown template %q", page)
	}
	p.Version = r.Version
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, block, p); err != nil {
		return fmt.Errorf("render %s/%s: %w", page, block, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := io.Copy(w, &buf)
	return err
}

// Number formats with a German thousands separator.
func Number(n int) string {
	s := strconv.Itoa(max(n, -n))
	var b strings.Builder
	if n < 0 {
		b.WriteByte('-')
	}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// IsMinutes reports a unit shown with the minute sign, like 12′.
func IsMinutes(unit string) bool {
	switch strings.ToLower(strings.TrimSuffix(unit, ".")) {
	case "min", "minute", "minuten":
		return true
	}
	return false
}

// dash renders stroke-dasharray for a ring of the given radius.
func dash(radius float64, percent int) string {
	circumference := 2 * math.Pi * radius
	filled := circumference * float64(min(max(percent, 0), 100)) / 100
	return fmt.Sprintf("%.2f %.2f", filled, circumference-filled)
}

// StreakLabel formats a streak: "27 Tage" or "5 Wochen".
func StreakLabel(s game.Streak) string {
	if s.Monthly {
		if s.Current == 1 {
			return "1 Monat"
		}
		return fmt.Sprintf("%d Monate", s.Current)
	}
	if s.Weekly {
		if s.Current == 1 {
			return "1 Woche"
		}
		return fmt.Sprintf("%d Wochen", s.Current)
	}
	if s.Current == 1 {
		return "1 Tag"
	}
	return fmt.Sprintf("%d Tage", s.Current)
}

// StreakUnit names the unit of a streak.
func StreakUnit(s game.Streak) string {
	switch {
	case s.Weekly:
		return "Wochen"
	case s.Monthly:
		return "Monate"
	}
	return "Tage"
}

// MultiplierLabel formats a bonus factor: "×1,5".
func MultiplierLabel(m float64) string {
	return "×" + strings.ReplaceAll(strconv.FormatFloat(m, 'f', -1, 64), ".", ",")
}

// LongDate formats "Freitag, 30. Oktober".
func LongDate(d model.Day) string {
	return fmt.Sprintf("%s, %d. %s", weekdays[d.WeekdayIndex()], d.Dom(), Months[d.Month()-1])
}

// ShortDate formats "30.10.2026".
func ShortDate(t time.Time) string { return t.Format("02.01.2006") }

// FrequencyLabel describes the rhythm of a habit.
func FrequencyLabel(h model.Habit) string {
	switch h.Frequency {
	case model.FreqWeekly:
		return fmt.Sprintf("%d× pro Woche", h.WeeklyTarget)
	case model.FreqMonthly:
		return fmt.Sprintf("%d× pro Monat", h.WeeklyTarget)
	case model.FreqCustom:
		letters := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
		var picked []string
		for i, l := range letters {
			if h.DaysMask&(1<<i) != 0 {
				picked = append(picked, l)
			}
		}
		return strings.Join(picked, ", ")
	default:
		return "täglich"
	}
}

// dict builds a map for passing several values to a sub-template.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is no string", pairs[i])
		}
		out[key] = pairs[i+1]
	}
	return out, nil
}
