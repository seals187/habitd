package game

import (
	"testing"
	"time"

	"github.com/seals187/habitd/internal/model"
)

func questHabit(name, kind string, start model.Day) *HabitData {
	h := NewHabitData(model.Habit{Name: name, Kind: kind, Frequency: model.FreqDaily}, start)
	h.AreaKey = "lernen"
	return h
}

func clock(d model.Day, hour int) time.Time {
	return time.Date(d.Year(), d.Month(), d.Dom(), hour, 0, 0, 0, time.UTC)
}

func TestQuestUntilCountsOnlyEarlyCheckins(t *testing.T) {
	d := day("2026-10-28")
	a, b := questHabit("A", model.KindBuild, d-5), questHabit("B", model.KindBuild, d-5)
	w := &World{Today: d, Habits: []*HabitData{a, b}}
	q := QuestDef{Kind: QuestUntil, Count: 2, Until: "12:00", Reward: 1}
	a.Done[d] = clock(d, 9)
	b.Done[d] = clock(d, 15)
	if st := q.Eval(w, d); st.Complete || st.Done != 1 || st.Total != 2 {
		t.Fatalf("late check-in counted: %+v", st)
	}
	b.Done[d] = clock(d, 11)
	if st := q.Eval(w, d); !st.Complete {
		t.Fatalf("both early: %+v", st)
	}
	if st := q.Eval(w, d); st.Items[0].Note != "09:00" {
		t.Fatalf("note: %+v", st.Items[0])
	}
}

func TestQuestLeavesOutSkippedHabits(t *testing.T) {
	d := day("2026-10-31")
	a, b := questHabit("A", model.KindBuild, d-5), questHabit("B", model.KindBuild, d-5)
	w := &World{Today: d, Habits: []*HabitData{a, b}}
	q := QuestDef{ID: 7, Kind: QuestAll, Reward: 1}
	a.Done[d] = clock(d, 9)
	if st := q.Eval(w, d); st.Complete || st.Total != 2 {
		t.Fatalf("both count: %+v", st)
	}
	b.Skips[7] = true
	if st := q.Eval(w, d); !st.Complete || st.Total != 1 || len(st.Items) != 1 {
		t.Fatalf("skipped habit counted: %+v", st)
	}
	if st := (QuestDef{ID: 8, Kind: QuestAll, Reward: 1}).Eval(w, d); st.Total != 2 {
		t.Fatalf("a skip belongs to one quest: %+v", st)
	}
	a.Skips[7] = true
	if q.Eligible(w, d) {
		t.Fatal("a quest without habits cannot be drawn")
	}
}

func TestQuestAllCountsRelapses(t *testing.T) {
	d := day("2026-10-28")
	build, avoid := questHabit("A", model.KindBuild, d-5), questHabit("B", model.KindAvoid, d-5)
	w := &World{Today: d, Habits: []*HabitData{build, avoid}}
	q := QuestDef{Kind: QuestAll}
	build.Done[d] = clock(d, 9)
	if !q.Eval(w, d).Complete {
		t.Fatal("clean avoid habit must not block")
	}
	avoid.Relapses = []model.Day{d}
	if q.Eval(w, d).Complete {
		t.Fatal("a relapse blocks it")
	}
	if !q.Eligible(w, d) {
		t.Fatal("eligible with a build habit")
	}
	if q.Eligible(&World{Today: d, Habits: []*HabitData{avoid}}, d) {
		t.Fatal("not without a build habit")
	}
}

func TestQuestCounterPercent(t *testing.T) {
	d := day("2026-10-28")
	h := questHabit("Wasser", model.KindBuild, d-5)
	h.Measure, h.Target, h.Unit = model.MeasureAmount, 3, "L"
	w := &World{Today: d, Habits: []*HabitData{h}}
	q := QuestDef{Kind: QuestCounter, Percent: 120}
	h.Entries[d] = 3
	if q.Eval(w, d).Complete {
		t.Fatal("100 percent is not 120")
	}
	h.Entries[d] = 3.6
	if !q.Eval(w, d).Complete {
		t.Fatal("120 percent reached")
	}
	plain := &World{Today: d, Habits: []*HabitData{questHabit("A", model.KindBuild, d-5)}}
	if q.Eligible(plain, d) {
		t.Fatal("needs a counter habit")
	}
}

func TestQuestCatchUp(t *testing.T) {
	d := day("2026-10-28")
	h := questHabit("A", model.KindBuild, d-5)
	w := &World{Today: d, Habits: []*HabitData{h}}
	q := QuestDef{Kind: QuestCatchUp}
	if !q.Eligible(w, d) {
		t.Fatal("missed yesterday")
	}
	h.Done[d] = clock(d, 9)
	if !q.Eval(w, d).Complete {
		t.Fatal("done today")
	}
	h.Done[d-1] = clock(d-1, 9)
	if q.Eligible(w, d) {
		t.Fatal("nothing to catch up")
	}
}

func TestQuestAvoidIsDecidedAfterTheDay(t *testing.T) {
	d := day("2026-10-28")
	h := questHabit("A", model.KindAvoid, d-5)
	q := QuestDef{Kind: QuestAvoid}
	if q.Eval(&World{Today: d, Habits: []*HabitData{h}}, d).Complete {
		t.Fatal("open until the day is over")
	}
	next := &World{Today: d + 1, Habits: []*HabitData{h}}
	if !q.Eval(next, d).Complete {
		t.Fatal("clean day")
	}
	h.Relapses = []model.Day{d}
	if q.Eval(next, d).Complete {
		t.Fatal("relapse")
	}
}

func TestQuestWeakestArea(t *testing.T) {
	d := day("2026-10-28")
	good, weak := questHabit("A", model.KindBuild, d-5), questHabit("B", model.KindBuild, d-5)
	weak.AreaKey = "familie"
	good.Done[d-1] = clock(d-1, 9)
	good.Done[d-2] = clock(d-2, 9)
	w := &World{Today: d, Habits: []*HabitData{good, weak}}
	st := QuestDef{Kind: QuestArea, Count: 1}.Eval(w, d)
	if len(st.Items) != 1 || st.Items[0].Name != "B" {
		t.Fatalf("items: %+v", st.Items)
	}
}

func TestQuestOffered(t *testing.T) {
	saturday := day("2026-10-31")
	h := questHabit("A", model.KindBuild, saturday-5)
	w := &World{Today: saturday, Habits: []*HabitData{h}, Level: 3}
	if (QuestDef{Days: 31}).Offered(w, saturday) {
		t.Fatal("Mo-Fr on a Saturday")
	}
	if !(QuestDef{Days: AllDays}).Offered(w, saturday) {
		t.Fatal("every day")
	}
	if (QuestDef{Days: AllDays, MinHabits: 3}).Offered(w, saturday) {
		t.Fatal("needs three habits")
	}
	if (QuestDef{Days: AllDays, MinLevel: 5}).Offered(w, saturday) {
		t.Fatal("level too low")
	}
	if (QuestDef{Days: AllDays, OnlyArea: "familie"}).Offered(w, saturday) {
		t.Fatal("area missing")
	}
}

func TestQuestLabel(t *testing.T) {
	q := QuestDef{Text: "Erledige {{anzahl}} Habits vor {{uhrzeit}} Uhr", Count: 3, Until: "12:00", RewardKind: RewardJoker, Reward: 1}
	if got := q.Label(); got != "Erledige 3 Habits vor 12:00 Uhr" {
		t.Fatal(got)
	}
	if got := q.RewardLabel(2); got != "+2 Joker" {
		t.Fatal(got)
	}
}
