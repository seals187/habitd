package server

import (
	"errors"
	"net/http"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/view"
)

// habitCard is a row on the Today screen.
type habitCard struct {
	ID       int64
	Name     string
	Area     model.Area
	XP       int
	Done     bool
	Relapsed bool
	Streak   game.Streak
}

type bridgeOffer struct {
	HabitID int64
	Habit   string
	Date    string
	Label   string
}

type todayData struct {
	Date     string
	Bal      model.Balances
	Progress game.Progress
	Title    string
	Quest    game.Quest
	QuestPay int
	Build    []habitCard
	Avoid    []habitCard
	Bridges  []bridgeOffer
}

func areaMap(areas []model.Area) map[int64]model.Area {
	out := make(map[int64]model.Area, len(areas))
	for _, a := range areas {
		out[a.ID] = a
	}
	return out
}

func (s *Server) todayData(r *http.Request) (*todayData, error) {
	q := s.svc.Store.Q(r.Context())
	w, err := s.svc.World(q)
	if err != nil {
		return nil, err
	}
	areas, err := q.Areas()
	if err != nil {
		return nil, err
	}
	byID := areaMap(areas)
	progress := game.ProgressFor(w.Balances.XP, w.Level)
	d := &todayData{
		Date: view.LongDate(w.Today), Bal: w.Balances, Progress: progress, Title: game.Title(progress.Level),
		Quest: game.QuestFor(w.Habits, w.Today), QuestPay: game.QuestCoins,
	}
	for _, h := range w.Habits {
		if h.Archived || w.Today < h.Start {
			continue
		}
		card := habitCard{ID: h.ID, Name: h.Name, Area: byID[h.AreaID], XP: game.BaseXP(h.Habit), Streak: h.StreakAt(w.Today)}
		if h.Kind == model.KindAvoid {
			for _, relapse := range h.Relapses {
				card.Relapsed = card.Relapsed || relapse == w.Today
			}
			d.Avoid = append(d.Avoid, card)
			continue
		}
		_, card.Done = h.Done[w.Today]
		if h.Frequency != model.FreqCustom || h.Scheduled(w.Today) || card.Done {
			d.Build = append(d.Build, card)
		}
	}
	mode, err := q.Setting(service.KeyJokerMode, service.JokerAuto)
	if err != nil {
		return nil, err
	}
	if mode == service.JokerManual && w.Balances.Jokers > 0 {
		for _, b := range service.Bridgeable(w) {
			d.Bridges = append(d.Bridges, bridgeOffer{HabitID: b.Habit.ID, Habit: b.Habit.Name, Date: b.Day.String(), Label: view.LongDate(b.Day)})
		}
	}
	return d, nil
}

func (s *Server) today(w http.ResponseWriter, r *http.Request) error {
	fresh, err := s.needsOnboarding(r.Context())
	if err != nil {
		return err
	}
	if fresh {
		return redirect(w, r, "/onboarding")
	}
	d, err := s.todayData(r)
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "today", s.page(r, "Heute", "today", d))
}

// afterHabitChange answers a check-in: a pending celebration wins, otherwise
// the screen the request came from is updated in place.
func (s *Server) afterHabitChange(w http.ResponseWriter, r *http.Request, habitID int64) error {
	waiting, err := s.svc.Store.Q(r.Context()).HasCelebration()
	if err != nil {
		return err
	}
	if waiting {
		return redirect(w, r, "/celebrate")
	}
	if !isHTMX(r) {
		return redirect(w, r, "/")
	}
	if r.FormValue("from") == "detail" {
		p, err := s.detailData(r, habitID)
		if err != nil {
			return err
		}
		return s.view.Fragment(w, "habit", "habit-body", s.page(r, p.Habit.Name, "", p))
	}
	d, err := s.todayData(r)
	if err != nil {
		return err
	}
	return s.view.Fragment(w, "today", "today-body", s.page(r, "Heute", "today", d))
}

func (s *Server) habitCheck(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	if err := s.svc.Check(r.Context(), id, day); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

func (s *Server) habitRelapse(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	if err := s.svc.Relapse(r.Context(), id, day); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

func (s *Server) habitJoker(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	if err := s.svc.UseJoker(r.Context(), id, day); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

func (s *Server) habitAction(r *http.Request) (int64, model.Day, error) {
	id, err := pathID(r)
	if err != nil {
		return 0, 0, err
	}
	day, err := s.requestDay(r)
	return id, day, err
}

type celebrateData struct {
	Level     *service.LevelUp
	Badge     *game.Badge
	Color     string
	Coins     int
	Message   string
	Confetti  []int
	CSRFToken string
}

func (s *Server) celebrate(w http.ResponseWriter, r *http.Request) error {
	_, kind, payload, err := s.svc.Store.Q(r.Context()).NextCelebration()
	if errors.Is(err, db.ErrNotFound) {
		return redirect(w, r, "/")
	}
	if err != nil {
		return err
	}
	d, err := celebrationView(kind, payload)
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "celebrate", s.page(r, "Geschafft!", "", d))
}

func (s *Server) celebrateNext(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	id, _, _, err := q.NextCelebration()
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	if err == nil {
		if err := q.DeleteCelebration(id); err != nil {
			return err
		}
	}
	return redirect(w, r, "/celebrate")
}
