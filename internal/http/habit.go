package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/templates"
	"github.com/seals187/habitd/internal/view"
)

// maxGoal limits the daily goal of counter and quantity habits.
const maxGoal = 1_000_000

// iconChoice is an icon offered in the habit form. Icons of specific habits
// carry the tag some badges look for.
type iconChoice struct {
	Name string
	Tag  string
}

var habitIcons = []iconChoice{
	{"drop", "wasser"}, {"book", "lesen"}, {"dumb", "sport"}, {"lotus", "meditation"}, {"handheart", ""}, {"bulb", "idee"},
	{"run", "sport"}, {"bike", "velo"}, {"heart", ""}, {"phone", "anrufen"}, {"users", ""}, {"brief", ""},
	{"meal", ""}, {"moon", "schlaf"}, {"sun", "frueh"}, {"leaf", ""}, {"cup", ""}, {"ban", ""},
	{"eye", ""}, {"phoneoff", "handy"}, {"cartx", "impulskauf"}, {"wallet", ""}, {"recycle", ""}, {"target", ""},
	{"flame", ""}, {"handshake", ""}, {"cake", "geburtstag"}, {"home", ""}, {"teach", ""},
}

type dayOption struct {
	Bit     int
	Label   string
	Checked bool
}

type habitFormData struct {
	Habit    model.Habit
	New      bool
	Areas    []model.Area
	Icons    []iconChoice
	Days     []dayOption
	Freqs    []freqOption
	Expanded bool // all fields visible; a new habit starts with name and suggestions only
	Popular  []popularChip
	Total    int
	Error    string
	Config   string // JSON read by the Alpine preview
}

func (s *Server) habitForm(w http.ResponseWriter, r *http.Request, h model.Habit, status int, message string) error {
	areas, err := s.svc.Store.Q(r.Context()).Areas()
	if err != nil {
		return err
	}
	measured := h.Measured()
	if h.Measure == "" {
		h = h.Normalized()
	}
	d := habitFormData{Habit: h, New: h.ID == 0, Areas: areas, Icons: habitIcons, Error: message, Freqs: freqOptions(h), Popular: popularRows(areas), Total: len(templates.All())}
	d.Expanded = !d.New || status != http.StatusOK || h.Name != ""
	for i, l := range []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"} {
		d.Days = append(d.Days, dayOption{Bit: i, Label: l, Checked: h.DaysMask&(1<<i) != 0})
	}
	target, step := "8", "1"
	if measured {
		target, step = strconv.FormatFloat(h.Target, 'f', -1, 64), strconv.FormatFloat(h.Step, 'f', -1, 64)
	}
	config, err := json.Marshal(map[string]any{"kind": h.Kind, "difficulty": h.Difficulty, "measure": h.Measure,
		"target": target, "step": step, "edited": measured})
	if err != nil {
		return err
	}
	d.Config = string(config)
	title := "Habit bearbeiten"
	if d.New {
		title = "Neues Habit"
	}
	return s.view.Render(w, status, "habit_form", s.page(r, title, "", d))
}

func (s *Server) habitNew(w http.ResponseWriter, r *http.Request) error {
	areas, err := s.svc.Store.Q(r.Context()).Areas()
	if err != nil {
		return err
	}
	h := model.Habit{Kind: model.KindBuild, Frequency: model.FreqDaily, Difficulty: model.Easy, Icon: habitIcons[0].Name}
	if len(areas) > 0 {
		h.AreaID = areas[0].ID
	}
	if t, ok := templates.Find(r.FormValue("template")); ok {
		if area, ok := areasByKey(areas)[t.Area]; ok {
			h = t.Habit(area.ID, s.svc.Now())
		}
	}
	if name := strings.TrimSpace(r.FormValue("name")); name != "" && h.TemplateID == "" {
		h.Name = name
	}
	return s.habitForm(w, r, h, http.StatusOK, "")
}

func (s *Server) habitEdit(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	h, err := s.svc.Store.Q(r.Context()).Habit(id)
	if err != nil {
		return err
	}
	return s.habitForm(w, r, h, http.StatusOK, "")
}

func tagForIcon(icon string) (string, bool) {
	for _, c := range habitIcons {
		if c.Name == icon {
			return c.Tag, true
		}
	}
	return "", false
}

func parseHabitForm(r *http.Request, h *model.Habit) string {
	h.Name = strings.TrimSpace(r.FormValue("name"))
	h.Kind = r.FormValue("kind")
	h.Difficulty = r.FormValue("difficulty")
	h.Icon = r.FormValue("icon")
	h.AreaID, _ = strconv.ParseInt(r.FormValue("area"), 10, 64)
	h.Frequency, h.WeeklyTarget, h.DaysMask = model.FreqDaily, 0, 0
	switch r.FormValue("freq") {
	case "w1", "w2", "w3", "w4", "w5", "w6":
		h.Frequency, h.WeeklyTarget = model.FreqWeekly, int(r.FormValue("freq")[1]-'0')
	case "m1", "m2", "m3", "m4":
		h.Frequency, h.WeeklyTarget = model.FreqMonthly, int(r.FormValue("freq")[1]-'0')
	case "custom":
		h.Frequency = model.FreqCustom
		for _, v := range r.Form["days"] {
			if bit, err := strconv.Atoi(v); err == nil && bit >= 0 && bit < 7 {
				h.DaysMask |= 1 << bit
			}
		}
	}
	if h.Kind != model.KindAvoid {
		h.Kind = model.KindBuild
	} else {
		h.Frequency, h.WeeklyTarget, h.DaysMask = model.FreqDaily, 0, 0
	}
	if h.Difficulty != model.Medium && h.Difficulty != model.Hard {
		h.Difficulty = model.Easy
	}
	h.Measure, h.Unit = r.FormValue("measure"), strings.TrimSpace(r.FormValue("unit"))
	if h.Kind == model.KindAvoid {
		h.Measure = model.MeasureCheck
	}
	h.Target, _ = parseDecimal(r.FormValue("target"))
	h.Step, _ = parseDecimal(r.FormValue("step"))
	h.Target, h.Step = model.RoundAmount(h.Target), model.RoundAmount(h.Step)
	*h = h.Normalized()
	tag, ok := tagForIcon(h.Icon)
	if !ok {
		h.Icon = habitIcons[0].Name
		tag = habitIcons[0].Tag
	}
	h.Tag = tag
	if h.ID == 0 {
		h.TemplateID = r.FormValue("template")
	}
	if t, ok := templates.Find(h.TemplateID); ok {
		h.Tag = t.Tag // keeps area-specific badges working
	} else {
		h.TemplateID = ""
	}
	switch {
	case h.Name == "":
		return "Bitte gib einen Namen ein"
	case len(h.Name) > 80:
		return "Der Name ist zu lang"
	case h.Frequency == model.FreqCustom && h.DaysMask == 0:
		return "Bitte wähle mindestens einen Wochentag"
	case h.Target <= 0 || h.Target > maxGoal:
		return "Das Tagesziel muss über 0 und höchstens 1.000.000 sein"
	case h.Step <= 0 || h.Step > h.Target:
		return "Pro Klick muss über 0 und höchstens beim Tagesziel liegen"
	case utf8.RuneCountInString(h.Unit) > 16:
		return "Die Einheit ist zu lang"
	}
	return ""
}

func (s *Server) habitSave(w http.ResponseWriter, r *http.Request) error {
	var h model.Habit
	q := s.svc.Store.Q(r.Context())
	if r.PathValue("id") != "" {
		id, err := pathID(r)
		if err != nil {
			return err
		}
		if h, err = q.Habit(id); err != nil {
			return err
		}
	} else {
		h.CreatedAt = s.svc.Now()
	}
	if err := r.ParseForm(); err != nil {
		return badRequest("Ungültiges Formular")
	}
	if message := parseHabitForm(r, &h); message != "" {
		return s.habitForm(w, r, h, http.StatusUnprocessableEntity, message)
	}
	areas, err := q.Areas()
	if err != nil {
		return err
	}
	if _, ok := areaMap(areas)[h.AreaID]; !ok {
		return s.habitForm(w, r, h, http.StatusUnprocessableEntity, "Bitte wähle einen Bereich")
	}
	id, err := q.SaveHabit(h)
	if err != nil {
		return err
	}
	if err := s.svc.HabitSaved(r.Context()); err != nil {
		return err
	}
	if h.ID != 0 {
		return redirect(w, r, fmt.Sprintf("/habits/%d", id))
	}
	return redirect(w, r, "/")
}

type habitRow struct {
	model.Habit
	Area      model.Area
	Frequency string
}

func (s *Server) habitList(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	habits, err := q.Habits()
	if err != nil {
		return err
	}
	areas, err := q.Areas()
	if err != nil {
		return err
	}
	byID := areaMap(areas)
	var active, archived []habitRow
	for _, h := range habits {
		row := habitRow{Habit: h, Area: byID[h.AreaID], Frequency: view.FrequencyLabel(h)}
		if h.Archived {
			archived = append(archived, row)
		} else {
			active = append(active, row)
		}
	}
	return s.view.Render(w, http.StatusOK, "habits", s.page(r, "Habits verwalten", "profile", map[string]any{"Active": active, "Archived": archived}))
}

func (s *Server) habitArchive(w http.ResponseWriter, r *http.Request) error {
	return s.setArchived(w, r, true)
}

func (s *Server) habitRestore(w http.ResponseWriter, r *http.Request) error {
	return s.setArchived(w, r, false)
}

func (s *Server) setArchived(w http.ResponseWriter, r *http.Request, archived bool) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).SetHabitArchived(id, archived); err != nil {
		return err
	}
	if err := s.svc.HabitSaved(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/habits")
}

func (s *Server) habitDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.DeleteHabit(r.Context(), id); err != nil {
		return err
	}
	return redirect(w, r, "/habits")
}

func (s *Server) habitDeleteAll(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.DeleteAllHabits(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/habits")
}

func (s *Server) habitMove(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	err = s.svc.Store.Tx(r.Context(), func(q *db.Q) error { return q.MoveHabit(id, r.FormValue("dir") == "up") })
	if err != nil {
		return err
	}
	return redirect(w, r, "/habits")
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
