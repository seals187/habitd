package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
)

func fsRead(files fs.FS, name string) ([]byte, error) { return fs.ReadFile(files, name) }

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

type apiHabit struct {
	model.Habit
	Streak     int  `json:"streak"`
	StreakDays int  `json:"streak_days"`
	Weekly     bool `json:"streak_in_weeks"`
	Longest    int  `json:"longest_streak"`
	DoneToday  bool `json:"done_today"`
}

func (s *Server) apiHabits(w http.ResponseWriter, r *http.Request) error {
	world, err := s.svc.World(s.svc.Store.Q(r.Context()))
	if err != nil {
		return err
	}
	out := []apiHabit{}
	for _, h := range world.Habits {
		if h.Archived {
			continue
		}
		st := h.StreakAt(world.Today)
		_, done := h.Done[world.Today]
		out = append(out, apiHabit{Habit: h.Habit, Streak: st.Current, StreakDays: st.Days, Weekly: st.Weekly, Longest: st.Longest, DoneToday: done})
	}
	return writeJSON(w, http.StatusOK, out)
}

func (s *Server) apiCheckins(w http.ResponseWriter, r *http.Request) error {
	e, err := s.svc.Store.Q(r.Context()).DumpAll()
	if err != nil {
		return err
	}
	from, to := r.FormValue("from"), r.FormValue("to")
	out := []db.ExportCheckin{}
	for _, c := range e.Checkins {
		if (from == "" || c.Date >= from) && (to == "" || c.Date <= to) {
			out = append(out, c)
		}
	}
	return writeJSON(w, http.StatusOK, out)
}

// apiCheck toggles a check-in, like the button in the app.
func (s *Server) apiCheck(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	if err := s.svc.Check(r.Context(), id, day); err != nil {
		return err
	}
	world, err := s.svc.World(s.svc.Store.Q(r.Context()))
	if err != nil {
		return err
	}
	h := service.Find(world, id)
	_, done := h.Done[day]
	return writeJSON(w, http.StatusOK, map[string]any{"date": day.String(), "done": done, "streak": h.StreakAt(world.Today).Current})
}

func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	world, err := s.svc.World(q)
	if err != nil {
		return err
	}
	unlocked, err := q.UnlockedBadges()
	if err != nil {
		return err
	}
	rules := s.svc.Rules()
	p := rules.ProgressFor(world.Balances.XP, world.Level)
	return writeJSON(w, http.StatusOK, map[string]any{
		"level": p.Level, "title": rules.Title(p.Level), "xp": world.Balances.XP, "xp_to_next": p.Missing,
		"coins": world.Balances.Coins, "coins_earned": world.Balances.CoinsEarned, "jokers": world.Balances.Jokers,
		"badges": len(unlocked), "badges_total": rules.ActiveBadges(), "redemptions": len(world.Redemptions),
	})
}
