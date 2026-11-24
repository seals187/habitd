package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// then is what a change of a habit sets off once it is written.
type then uint8

const (
	thenNothing then = iota
	thenBadges       // levels and badges, amount badges count every click
	thenAll          // the quest of today, then levels and badges
)

// onHabit runs a change of one habit in a transaction: it finds the habit in a
// fresh World, lets allow turn the change down, writes it through a dayWriter and
// settles what follows from it.
func (s *Service) onHabit(ctx context.Context, habitID int64, allow func(w *game.World, h *game.HabitData) error,
	change func(dw dayWriter) (*Trigger, then, error)) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		w, err := s.World(q)
		if err != nil {
			return err
		}
		h := Find(w, habitID)
		if h == nil {
			return db.ErrNotFound
		}
		if err := allow(w, h); err != nil {
			return err
		}
		trigger, next, err := change(dayWriter{s: s, q: q, w: w, h: h})
		if err != nil {
			return err
		}
		switch next {
		case thenAll:
			if err := s.updateQuest(q, w); err != nil {
				return err
			}
			fallthrough
		case thenBadges:
			return s.evaluate(q, trigger, false)
		}
		return nil
	})
}

// dayWriter changes the days of one habit. Every write lands in the store and in
// the habit of the World alike, so the later steps of the same transaction, such
// as the goal, the streak and the quest, see what was written.
type dayWriter struct {
	s *Service
	q *db.Q
	w *game.World
	h *game.HabitData
}

func (dw dayWriter) checked(d model.Day) bool {
	_, done := dw.h.Done[d]
	return done
}

// check books the check-in of a day with its XP, Taler and streak joker.
func (dw dayWriter) check(d model.Day) (*Trigger, error) { return dw.s.addCheckin(dw.q, dw.w, dw.h, d) }

// uncheck takes the check-in of a day back, and what it earned.
func (dw dayWriter) uncheck(d model.Day) error {
	if err := dw.q.DeleteCheckin(dw.h.ID, d); err != nil {
		return err
	}
	delete(dw.h.Done, d)
	return dw.s.reverse(dw.q, checkinRef(dw.h.ID, d))
}

// stamp moves the check-in of a day to the time it was really reached.
func (dw dayWriter) stamp(d model.Day, at time.Time) error {
	if at.IsZero() || !dw.checked(d) {
		return nil
	}
	if err := dw.q.Checkin(model.Checkin{HabitID: dw.h.ID, Date: d, CreatedAt: at}); err != nil {
		return err
	}
	dw.h.Done[d] = at
	return nil
}

// add books an amount on a day.
func (dw dayWriter) add(d model.Day, amount float64, at time.Time) error {
	if err := dw.q.AddEntry(model.Entry{HabitID: dw.h.ID, Date: d, Amount: amount, CreatedAt: at}); err != nil {
		return err
	}
	dw.h.Entries[d] = model.RoundAmount(dw.h.Entries[d] + amount)
	return nil
}

// clear removes all amounts of a day.
func (dw dayWriter) clear(d model.Day) error {
	if err := dw.q.DeleteDayEntries(dw.h.ID, d); err != nil {
		return err
	}
	dw.h.Entries[d] = 0
	return nil
}

// undoLast removes the last amount booked on a day.
func (dw dayWriter) undoLast(d model.Day) error {
	if err := dw.q.DeleteLastEntry(dw.h.ID, d); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrNotAllowed
		}
		return err
	}
	totals, err := dw.q.DayTotals(d)
	if err != nil {
		return err
	}
	dw.h.Entries[d] = totals[dw.h.ID]
	return nil
}

// fill tops a counter or quantity habit up to its goal, for days marked done in
// the calendar instead of on the card.
func (dw dayWriter) fill(d model.Day) error {
	if !dw.h.Measured() {
		return nil
	}
	if missing := model.RoundAmount(dw.h.Target - dw.h.Entries[d]); missing > 0 {
		return dw.add(d, missing, dw.s.atDay(d))
	}
	return nil
}

// syncGoal aligns the check-in with the amounts: reaching the goal adds it,
// falling below takes it back. It reports whether the check-in changed.
func (dw dayWriter) syncGoal(d model.Day) (*Trigger, bool, error) {
	day, done := dw.h.GoalCheckin(d)
	met := dw.h.GoalTotal(d) >= dw.h.Target
	switch {
	case met && !done:
		trigger, err := dw.check(d)
		return trigger, true, err
	case done && !met:
		return nil, true, dw.uncheck(day)
	}
	return nil, false, nil
}

// toggleRelapse records or takes back a relapse.
func (dw dayWriter) toggleRelapse(d model.Day) error {
	h := dw.h
	if i := slices.Index(h.Relapses, d); i >= 0 {
		if err := dw.q.DeleteRelapse(h.ID, d); err != nil {
			return err
		}
		h.Relapses = slices.Delete(h.Relapses, i, i+1)
		return nil
	}
	if err := dw.q.AddRelapse(h.ID, d); err != nil {
		return err
	}
	h.Relapses = append(h.Relapses, d)
	slices.Sort(h.Relapses)
	return nil
}

// Check toggles a check-in of a build habit, including XP and Taler.
func (s *Service) Check(ctx context.Context, habitID int64, d model.Day) error {
	if err := s.inRange(d); err != nil {
		return err
	}
	allow := func(_ *game.World, h *game.HabitData) error {
		if h.Kind != model.KindBuild || h.Archived {
			return ErrNotAllowed
		}
		return nil
	}
	return s.onHabit(ctx, habitID, allow, func(dw dayWriter) (*Trigger, then, error) {
		switch {
		case dw.h.GoalWeek: // days are parts of the week, so a tap never fills the whole goal
			var err error
			if dw.h.Entries[d] > 0 {
				err = dw.clear(d)
			} else {
				err = dw.add(d, dw.h.Step, s.atDay(d))
			}
			if err != nil {
				return nil, thenNothing, err
			}
			trigger, _, err := dw.syncGoal(d)
			return trigger, thenAll, err
		case dw.checked(d):
			if err := dw.uncheck(d); err != nil {
				return nil, thenNothing, err
			}
			return nil, thenAll, dw.clear(d)
		}
		if err := dw.fill(d); err != nil {
			return nil, thenNothing, err
		}
		trigger, err := dw.check(d)
		return trigger, thenAll, err
	})
}

// DayEdit says how a day of a habit should look after it was edited by hand.
type DayEdit struct {
	Done   bool      // reached, for habits without an amount
	Amount float64   // the sum of the day, for counters and quantities
	At     time.Time // when it was reached, the zero time keeps what is stored
}

// DayEditable reports whether a day can be opened in the calendar: every day
// since the start of the habit, and the usual days of backfill before it.
func DayEditable(h *game.HabitData, d, today model.Day) bool {
	return h.Kind == model.KindBuild && !h.Archived && d <= today && (d >= h.Since() || d >= today-game.BackfillDays)
}

// DayTime is a time of a day entered by hand. The second tells it from noon, which
// stands for a day filled in without a time.
func (s *Service) DayTime(d model.Day, hour, minute int) time.Time {
	return time.Date(d.Year(), d.Month(), d.Dom(), hour, minute, 1, 0, s.Loc)
}

// EditDay sets the state of a day, its amount and its time. XP, Taler, streaks and
// badges follow as they do for a click on that day.
func (s *Service) EditDay(ctx context.Context, habitID int64, d model.Day, edit DayEdit) error {
	if edit.At.Truncate(time.Minute).After(s.Now()) {
		return ErrFuture
	}
	allow := func(w *game.World, h *game.HabitData) error {
		if !DayEditable(h, d, w.Today) {
			return ErrNotAllowed
		}
		return nil
	}
	return s.onHabit(ctx, habitID, allow, func(dw dayWriter) (*Trigger, then, error) {
		var trigger *Trigger
		var err error
		switch {
		case dw.h.Measured():
			trigger, err = s.editAmount(dw, d, edit)
		case edit.Done && !dw.checked(d):
			trigger, err = dw.check(d)
		case !edit.Done && dw.checked(d):
			err = dw.uncheck(d)
		}
		if err != nil {
			return nil, thenNothing, err
		}
		return trigger, thenAll, dw.stamp(d, edit.At)
	})
}

// editAmount replaces the amounts of a day by their new sum. An unchanged sum keeps
// its clicks, only the last one moves to the new time.
func (s *Service) editAmount(dw dayWriter, d model.Day, edit DayEdit) (*Trigger, error) {
	amount := model.RoundAmount(edit.Amount)
	if amount == dw.h.Entries[d] {
		if amount == 0 || edit.At.IsZero() {
			return nil, nil
		}
		return nil, dw.q.MoveLastEntry(dw.h.ID, d, edit.At)
	}
	if err := dw.clear(d); err != nil {
		return nil, err
	}
	if amount > 0 {
		at := edit.At
		if at.IsZero() {
			at = s.atDay(d)
		}
		if err := dw.add(d, amount, at); err != nil {
			return nil, err
		}
	}
	trigger, _, err := dw.syncGoal(d)
	return trigger, err
}

// takesEntries allows counter and quantity habits that can take amounts.
func takesEntries(_ *game.World, h *game.HabitData) error {
	if !h.Measured() || h.Kind != model.KindBuild || h.Archived {
		return ErrNotAllowed
	}
	return nil
}

// AddEntry books one click. The check-in with XP, streak and badges follows
// when the click reaches the daily goal.
func (s *Service) AddEntry(ctx context.Context, habitID int64) error {
	return s.onHabit(ctx, habitID, takesEntries, func(dw dayWriter) (*Trigger, then, error) {
		d := dw.w.Today
		if err := dw.add(d, dw.h.Step, s.Now()); err != nil {
			return nil, thenNothing, err
		}
		trigger, changed, err := dw.syncGoal(d)
		if !changed {
			return nil, thenBadges, err
		}
		return trigger, thenAll, err
	})
}

// UndoEntry takes back the last click of today, for a weekly goal the last click
// of the week. Falling below the goal also takes back the check-in.
func (s *Service) UndoEntry(ctx context.Context, habitID int64) error {
	return s.onHabit(ctx, habitID, takesEntries, func(dw dayWriter) (*Trigger, then, error) {
		d := dw.w.Today
		for dw.h.GoalWeek && d > d.WeekStart() && dw.h.Entries[d] == 0 {
			d--
		}
		if err := dw.undoLast(d); err != nil {
			return nil, thenNothing, err
		}
		_, changed, err := dw.syncGoal(d)
		if !changed {
			return nil, thenNothing, err
		}
		return nil, thenAll, err
	})
}

// Relapse toggles a relapse of an avoid habit. XP are never taken away.
func (s *Service) Relapse(ctx context.Context, habitID int64, d model.Day) error {
	if err := s.inRange(d); err != nil {
		return err
	}
	allow := func(_ *game.World, h *game.HabitData) error {
		if h.Kind != model.KindAvoid || h.Archived {
			return ErrNotAllowed
		}
		return nil
	}
	return s.onHabit(ctx, habitID, allow, func(dw dayWriter) (*Trigger, then, error) {
		return nil, thenAll, dw.toggleRelapse(d)
	})
}

// UseJoker bridges a missed day by hand.
func (s *Service) UseJoker(ctx context.Context, habitID int64, d model.Day) error {
	if err := s.inRange(d); err != nil {
		return err
	}
	allow := func(w *game.World, h *game.HabitData) error {
		_, done := h.Done[d]
		switch {
		case w.Balances.Jokers <= 0:
			return ErrJokerLimit
		case d >= w.Today, done, h.Joker[d], h.Kind != model.KindBuild:
			return ErrNotAllowed
		}
		return nil
	}
	return s.onHabit(ctx, habitID, allow, func(dw dayWriter) (*Trigger, then, error) {
		return nil, thenNothing, s.bridge(dw.q, dw.h, d)
	})
}
