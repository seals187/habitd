package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/view"
)

// daySheetData is the sheet to edit one day of a habit from the month view.
type daySheetData struct {
	ID       int64
	Name     string
	Title    string
	Date     string
	View     string // the view to come back to
	Week     string
	Month    string
	Measured bool
	Label    string // what the amount counts
	Goal     string
	Amount   string
	Time     string
}

// plainAmount writes an amount for a number field, which wants a decimal point.
func plainAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// dayClock finds the time a day was reached, as far as it is known.
func dayClock(q *db.Q, h *game.HabitData, d model.Day) (string, error) {
	if at, done := h.Done[d]; done && realMoment(at, d) {
		return at.Format("15:04"), nil
	}
	if !h.Measured() {
		return "", nil
	}
	entries, err := q.HabitEntries(h.ID, d)
	if err != nil {
		return "", err
	}
	clock := ""
	for _, e := range entries {
		if e.Date == d && realMoment(e.CreatedAt, d) {
			clock = e.CreatedAt.Format("15:04")
		}
	}
	return clock, nil
}

// newDaySheet fills the sheet. A day without an entry offers the goal, so saving is
// all it takes to mark it as reached.
func (s *Server) newDaySheet(q *db.Q, h *game.HabitData, d, today model.Day) (*daySheetData, error) {
	clock, err := dayClock(q, h, d)
	if err != nil {
		return nil, err
	}
	if clock == "" && d == today {
		clock = s.svc.Now().In(s.svc.Loc).Format("15:04")
	}
	sheet := &daySheetData{ID: h.ID, Name: h.Name, Title: view.FullDate(d), Date: d.String(), Measured: h.Measured(), Time: clock}
	if !h.Measured() {
		return sheet, nil
	}
	unit := amountUnitOf(h.Unit)
	sheet.Label, sheet.Goal = unit.name, goalWord(h)+" "+unit.label(h.Target)
	if sheet.Label == "" {
		sheet.Label = "Anzahl"
	}
	amount := h.Entries[d]
	if amount == 0 {
		amount = h.Target
		if h.GoalWeek { // a single day only adds to the goal of the week
			amount = h.Step
		}
	}
	sheet.Amount = plainAmount(amount)
	return sheet, nil
}

func (s *Server) daySheet(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	q := s.svc.Store.Q(r.Context())
	world, err := s.svc.World(q)
	if err != nil {
		return err
	}
	h := service.Find(world, id)
	if h == nil {
		return db.ErrNotFound
	}
	if !service.DayEditable(h, day, world.Today) {
		return service.ErrNotAllowed
	}
	sheet, err := s.newDaySheet(q, h, day, world.Today)
	if err != nil {
		return err
	}
	sheet.View, sheet.Week, sheet.Month = r.FormValue("view"), r.FormValue("w"), r.FormValue("m")
	return s.view.Fragment(w, "habit", "day-sheet", s.page(r, h.Name, "", sheet))
}

// readDayEdit turns the form into an edit. An empty time keeps the stored one.
func (s *Server) readDayEdit(r *http.Request, d model.Day) (service.DayEdit, error) {
	edit := service.DayEdit{Done: r.FormValue("done") != ""}
	if raw := r.FormValue("amount"); raw != "" {
		amount, err := parseDecimal(raw)
		if err != nil || amount < 0 {
			return edit, badRequest("Ungültiger Wert")
		}
		edit.Amount = amount
	}
	if raw := r.FormValue("time"); raw != "" {
		t, err := time.Parse("15:04", raw)
		if err != nil {
			return edit, badRequest("Ungültige Uhrzeit")
		}
		edit.At = s.svc.DayTime(d, t.Hour(), t.Minute())
	}
	return edit, nil
}

func (s *Server) dayEdit(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	edit, err := s.readDayEdit(r, day)
	if err != nil {
		return err
	}
	if err := s.svc.EditDay(r.Context(), id, day, edit); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}
