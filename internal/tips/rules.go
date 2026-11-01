package tips

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// Tones of the icons, they are CSS classes.
const (
	toneAccent = "accent"
	toneGold   = "gold"
	toneOrange = "orange"
)

// How far rules look back and what makes them speak up.
const (
	weekdayDays   = 56   // eight of each weekday
	weekdayMin    = 2    // a weekday needs this many planned days to say anything
	weekdayGap    = 0.15 // lead on the average that makes a strong day
	weakGap       = 0.25 // distance to the average that makes a weak day
	trendShare    = 0.05 // change of the daily amount, in shares of the goal
	morningLow    = 0.25 // share before noon that counts as little
	earlyShare    = 0.6  // share before noon that counts as early
	nearGoal      = 0.85 // share of the goal that counts as almost there
	nearDays      = 14
	nearMin       = 2
	aboveShare    = 1.1
	steadyDays    = 30
	steadyPlan    = 7
	steadyShare   = 0.9
	riskStreak    = 3
	riskLate      = 20 * 60 // from here on any open day is at risk
	riskWindow    = 60      // minutes past the usual time
	recordMin     = 3
	recordGap     = 3
	recordRun     = 7
	relapseDays   = 90
	relapseMin    = 3
	relapseSame   = 2
	relapseShare  = 0.4
	relapseToday  = 25 // the risky day is today, so this is what matters now
	roundMinutes  = 30
	favoriteMin   = 5
	favoriteCount = 2
)

var weekdays = [7]string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}

// cleanMarks are the streaks of an avoid habit worth running towards.
var cleanMarks = []int{14, 30, 60, 90, 180, 365, 500, 1000}

func clock(minutes int) string { return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60) }

// median is the usual time of the stamps in minutes of the day.
func median(stamps []Stamp) int {
	times := make([]int, len(stamps))
	for i, s := range stamps {
		times[i] = s.minutes()
	}
	slices.Sort(times)
	return times[len(times)/2]
}

// usualTime rounds the median to something one can remember.
func usualTime(stamps []Stamp) int {
	return (median(stamps) + roundMinutes/2) / roundMinutes * roundMinutes
}

func unitWord(s game.Streak, n int) string {
	word := func(one, many string) string {
		if n == 1 {
			return one
		}
		return many
	}
	switch {
	case s.Monthly:
		return word("Monat", "Monate")
	case s.Weekly:
		return word("Woche", "Wochen")
	}
	return word("Tag", "Tage")
}

// dativeWord is unitWord after a preposition: von 12 Tagen.
func dativeWord(s game.Streak, n int) string {
	word := unitWord(s, n)
	switch {
	case n == 1:
		return word
	case s.Monthly:
		return word + "n"
	case s.Weekly:
		return word
	}
	return word + "n"
}

// plural picks the word that fits a number.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// typicalTime names when the goal is usually reached.
func typicalTime(in Input) (Tip, bool) {
	if len(in.Done) < MinEntries {
		return Tip{}, false
	}
	verb := "erledigt"
	if in.Habit.Measured() {
		verb = "erreicht"
	}
	tip := Tip{Icon: "clock", Tone: toneAccent, Title: fmt.Sprintf("Meist %s gegen %s", verb, clock(usualTime(in.Done))), Score: 60}
	reached, plan := 0, 0
	var first *Stamp
	for d := in.Today - 6; d <= in.Today; d++ {
		if in.Habit.Scheduled(d) {
			plan++
		}
		if in.done(d) {
			reached++
		}
	}
	for i, s := range in.Done {
		if s.Day > in.Today-7 && (first == nil || s.minutes() < first.minutes()) {
			first = &in.Done[i]
		}
	}
	if plan > 0 {
		tip.Detail = fmt.Sprintf("an %d von %d Tagen", reached, plan)
	} else {
		tip.Detail = fmt.Sprintf("%d-mal in den letzten 7 Tagen", reached)
	}
	if first != nil {
		tip.Detail += fmt.Sprintf(", am frühesten %s %s", weekdays[first.Day.WeekdayIndex()], clock(first.minutes()))
	}
	return tip, true
}

// weekdayRate is how often the planned days of each weekday worked out.
type weekdayRate struct {
	Day        int
	Done, Plan int
}

func (r weekdayRate) share() float64 { return float64(r.Done) / float64(r.Plan) }

// weekdayRates lists the weekdays with enough days, and the rate across all of them.
func weekdayRates(in Input) ([]weekdayRate, float64) {
	rates := make([]weekdayRate, 7)
	for i := range rates {
		rates[i].Day = i
	}
	for d := max(in.Today-weekdayDays, in.Habit.Start); d < in.Today; d++ {
		if !in.Habit.Scheduled(d) {
			continue
		}
		r := &rates[d.WeekdayIndex()]
		r.Plan++
		if in.done(d) {
			r.Done++
		}
	}
	rates = slices.DeleteFunc(rates, func(r weekdayRate) bool { return r.Plan < weekdayMin })
	done, plan := 0, 0
	for _, r := range rates {
		done += r.Done
		plan += r.Plan
	}
	if plan == 0 {
		return nil, 0
	}
	return rates, float64(done) / float64(plan)
}

// bestWeekday praises the weekday that works best.
func bestWeekday(in Input) (Tip, bool) {
	if !in.daily() || !in.enoughDays() {
		return Tip{}, false
	}
	rates, average := weekdayRates(in)
	if len(rates) < 2 {
		return Tip{}, false
	}
	best := slices.MaxFunc(rates, func(a, b weekdayRate) int { return cmp.Compare(a.share(), b.share()) })
	if best.share()-average < weekdayGap {
		return Tip{}, false
	}
	name := weekdays[best.Day]
	return Tip{Icon: "star", Tone: toneGold, Title: name + " ist dein stärkster Tag",
		Detail: fmt.Sprintf("an %d von %d %s geschafft", best.Done, best.Plan, name), Score: 40}, true
}

// weakWeekday points at the weekday with the most room.
func weakWeekday(in Input) (Tip, bool) {
	if !in.daily() || !in.enoughDays() {
		return Tip{}, false
	}
	rates, average := weekdayRates(in)
	if len(rates) < 2 {
		return Tip{}, false
	}
	weak := slices.MinFunc(rates, func(a, b weekdayRate) int { return cmp.Compare(a.share(), b.share()) })
	if average-weak.share() < weakGap {
		return Tip{}, false
	}
	name := weekdays[weak.Day]
	return Tip{Icon: "calendar", Tone: toneGold, Title: "Am " + name + " ist am meisten drin",
		Detail: fmt.Sprintf("bisher %d von %d – eine feste Uhrzeit hilft", weak.Done, weak.Plan), Score: 50}, true
}

// weekSum adds up seven days. Only days that are over count, today is still open.
func weekSum(in Input, last model.Day) float64 {
	sum := 0.0
	for d := last - 6; d <= last; d++ {
		switch {
		case in.Habit.Measured():
			sum += in.Habit.Entries[d]
		case in.done(d):
			sum++
		}
	}
	return sum
}

// trend compares the last seven finished days with the seven before.
func trend(in Input) (Tip, bool) {
	h := in.Habit
	if in.avoid() || (!in.daily() && !h.Measured()) || int(in.Today-h.Start) < 14 {
		return Tip{}, false
	}
	now, before := weekSum(in, in.Today-1), weekSum(in, in.Today-8)
	if h.Measured() {
		diff := (now - before) / 7
		if math.Abs(diff) < h.Target*trendShare {
			return Tip{}, false
		}
		if diff > 0 {
			return Tip{Icon: "arrow-up", Tone: toneAccent, Title: "+" + in.amount(diff) + " pro Tag mehr", Detail: "als in der Vorwoche", Score: 45}, true
		}
		return Tip{Icon: "arrow-up", Tone: toneAccent, Title: "Da geht wieder mehr",
			Detail: "in der Vorwoche waren es " + in.amount(-diff) + " pro Tag mehr", Score: 35}, true
	}
	diff := int(now - before)
	switch {
	case diff > 0:
		return Tip{Icon: "arrow-up", Tone: toneAccent, Title: fmt.Sprintf("+%d %s mehr", diff, plural(diff, "Tag", "Tage")), Detail: "als in der Vorwoche", Score: 45}, true
	case diff < 0:
		return Tip{Icon: "arrow-up", Tone: toneAccent, Title: "Da geht wieder mehr",
			Detail: fmt.Sprintf("in der Vorwoche %s es %d %s mehr", plural(-diff, "war", "waren"), -diff, plural(-diff, "Tag", "Tage")), Score: 35}, true
	}
	return Tip{}, false
}

// streakAtRisk speaks up when a running streak still waits for today.
func streakAtRisk(in Input) (Tip, bool) {
	s := in.Streak
	if in.avoid() || !in.enoughDays() || s.Current < riskStreak || !in.Habit.DueOn(in.Today) || in.done(in.Today) {
		return Tip{}, false
	}
	now := in.Now.Hour()*60 + in.Now.Minute()
	due := riskLate
	if len(in.Done) >= MinEntries {
		due = min(median(in.Done)+riskWindow, riskLate)
	}
	if now < due {
		return Tip{}, false
	}
	tip := Tip{Icon: "flame", Tone: toneOrange, Title: fmt.Sprintf("Deine Serie von %d %s wartet", s.Current, dativeWord(s, s.Current)), Score: 90}
	tip.Detail = "heute reicht ein kleiner Schritt"
	if h := in.Habit; h.Measured() && !h.GoalWeek {
		tip.Detail = "noch " + in.amount(h.Target-h.Entries[in.Today]) + " bis zum Ziel von heute"
	}
	return tip, true
}

// recordInReach cheers on a streak close to or at the record.
func recordInReach(in Input) (Tip, bool) {
	s := in.Streak
	gap := s.Longest - s.Current
	switch {
	case !in.enoughDays() || s.Current < recordMin:
		return Tip{}, false
	case gap <= 0 && s.Current >= recordRun:
		return Tip{Icon: "trophy", Tone: toneGold, Title: "Du bist auf Rekordkurs",
			Detail: fmt.Sprintf("%d %s sind deine längste Serie", s.Current, unitWord(s, s.Current)), Score: 70}, true
	case gap > 0 && gap <= recordGap:
		return Tip{Icon: "trophy", Tone: toneGold, Title: fmt.Sprintf("Noch %d %s bis zum Rekord", gap, unitWord(s, gap)),
			Detail: fmt.Sprintf("dein Rekord liegt bei %d %s", s.Longest, dativeWord(s, s.Longest)), Score: 80}, true
	}
	return Tip{}, false
}

// morningShare shows how little of the amount happens before noon.
func morningShare(in Input) (Tip, bool) {
	if len(in.Stamps) < MinEntries {
		return Tip{}, false
	}
	morning, total := 0.0, 0.0
	for _, s := range in.Stamps {
		total += s.Amount
		if s.minutes() < noon {
			morning += s.Amount
		}
	}
	if total <= 0 || morning/total >= morningLow {
		return Tip{}, false
	}
	share := int(math.Round(morning * 100 / total))
	if in.Drink {
		return Tip{Icon: "sun", Tone: toneGold, Title: "Morgens trinkst du wenig",
			Detail: fmt.Sprintf("nur %d %% bis 12 Uhr – ein Glas zum Frühstück hilft", share), Score: 55}, true
	}
	return Tip{Icon: "sun", Tone: toneGold, Title: "Morgens ist noch Luft",
		Detail: fmt.Sprintf("nur %d %% bis 12 Uhr – ein früher Start nimmt den Druck", share), Score: 55}, true
}

// almostThere finds days that only missed the goal by a little.
func almostThere(in Input) (Tip, bool) {
	h := in.Habit
	if !in.enoughDays() {
		return Tip{}, false
	}
	near := 0
	for d := max(in.Today-nearDays, h.Start); d < in.Today; d++ {
		if v := h.Entries[d]; !in.done(d) && v >= h.Target*nearGoal && v < h.Target {
			near++
		}
	}
	if near < nearMin {
		return Tip{}, false
	}
	return Tip{Icon: "target", Tone: toneAccent, Title: "Oft fehlt nur ein kleines Stück",
		Detail: fmt.Sprintf("an %d Tagen fehlten weniger als %s", near, in.amount(h.Target*(1-nearGoal))), Score: 52}, true
}

// aboveGoal says so when the last days ran clearly past the goal.
func aboveGoal(in Input) (Tip, bool) {
	h := in.Habit
	if !in.enoughDays() {
		return Tip{}, false
	}
	average := weekSum(in, in.Today-1) / 7
	if average < h.Target*aboveShare {
		return Tip{}, false
	}
	return Tip{Icon: "sparkles", Tone: toneAccent, Title: "Du liegst über deinem Ziel",
		Detail: "im Schnitt " + in.amount(average) + " pro Tag", Score: 38}, true
}

// checkEarly notices when a tick mostly happens in the morning.
func checkEarly(in Input) (Tip, bool) {
	if len(in.Done) < MinEntries {
		return Tip{}, false
	}
	early := 0
	for _, s := range in.Done {
		if s.minutes() < noon {
			early++
		}
	}
	share := float64(early) / float64(len(in.Done))
	if share < earlyShare {
		return Tip{}, false
	}
	return Tip{Icon: "sunrise", Tone: toneGold, Title: "Du startest früh",
		Detail: fmt.Sprintf("%d %% schon bis 12 Uhr erledigt", int(math.Round(share*100))), Score: 30}, true
}

// checkSteady praises a reliable month.
func checkSteady(in Input) (Tip, bool) {
	if !in.daily() || !in.enoughDays() {
		return Tip{}, false
	}
	done, plan := 0, 0
	for d := max(in.Today-steadyDays, in.Habit.Start); d < in.Today; d++ {
		if in.Habit.Scheduled(d) {
			plan++
			if in.done(d) {
				done++
			}
		}
	}
	if plan < steadyPlan || float64(done)/float64(plan) < steadyShare {
		return Tip{}, false
	}
	return Tip{Icon: "check", Tone: toneAccent, Title: "Sehr verlässlich", Detail: fmt.Sprintf("an %d von %d Tagen geschafft", done, plan), Score: 35}, true
}

// avoidRun counts towards the next mark of a clean run.
func avoidRun(in Input) (Tip, bool) {
	days := in.Streak.Days
	if !in.avoid() || !in.enoughDays() || days < MinDays {
		return Tip{}, false
	}
	tip := Tip{Icon: "shield", Tone: toneAccent, Title: fmt.Sprintf("Schon %d Tage geschafft", days), Detail: "jeder Tag zählt weiter mit", Score: 45}
	for _, mark := range cleanMarks {
		if days < mark {
			tip.Detail = fmt.Sprintf("noch %d %s bis %d Tage", mark-days, plural(mark-days, "Tag", "Tage"), mark)
			break
		}
	}
	return tip, true
}

// avoidRiskyDay finds the weekday most relapses fall on.
func avoidRiskyDay(in Input) (Tip, bool) {
	if !in.avoid() {
		return Tip{}, false
	}
	var counts [7]int
	total := 0
	for _, d := range in.Habit.Relapses {
		if d > in.Today-relapseDays && d <= in.Today {
			counts[d.WeekdayIndex()]++
			total++
		}
	}
	top := 0
	for i, n := range counts {
		if n > counts[top] {
			top = i
		}
	}
	if total < relapseMin || counts[top] < relapseSame || float64(counts[top])/float64(total) < relapseShare {
		return Tip{}, false
	}
	tip := Tip{Icon: "calendar", Tone: toneOrange, Title: "Am " + weekdays[top] + " lohnt sich ein Plan",
		Detail: fmt.Sprintf("%d von %d Rückfällen – gönn dir dann etwas Schönes", counts[top], total), Score: 60}
	if in.Today.WeekdayIndex() == top {
		tip.Title = "Heute ist " + weekdays[top] + " – plan dir etwas Schönes"
		tip.Score += relapseToday
	}
	return tip, true
}

// weekPace tells where the goal of this week stands.
func weekPace(in Input) (Tip, bool) {
	if !in.weekly() {
		return Tip{}, false
	}
	h := in.Habit
	start := in.Today.WeekStart()
	left := int(start+6-in.Today) + 1
	if h.GoalWeek && h.Measured() {
		missing := h.Target - h.GoalTotal(in.Today)
		if missing <= 0 {
			return Tip{Icon: "target", Tone: toneAccent, Title: "Wochenziel geschafft", Detail: "alles Weitere ist Bonus", Score: 65}, true
		}
		return Tip{Icon: "target", Tone: toneAccent, Title: "Noch " + in.amount(missing) + " bis zum Wochenziel",
			Detail: fmt.Sprintf("%d %s dafür", left, plural(left, "Tag bleibt", "Tage bleiben")), Score: 70}, true
	}
	done := 0
	for d := start; d <= in.Today; d++ {
		if in.done(d) {
			done++
		}
	}
	missing := h.WeekTarget() - done
	switch {
	case missing <= 0:
		return Tip{Icon: "target", Tone: toneAccent, Title: "Wochenziel geschafft", Detail: fmt.Sprintf("%d von %d diese Woche", done, h.WeekTarget()), Score: 65}, true
	case in.done(in.Today):
		left--
	}
	if missing > left {
		return Tip{}, false
	}
	return Tip{Icon: "target", Tone: toneAccent, Title: fmt.Sprintf("Noch %d bis zum Wochenziel", missing),
		Detail: fmt.Sprintf("%d %s dafür", left, plural(left, "Tag bleibt", "Tage bleiben")), Score: 70}, true
}

// weekFavorites names the weekdays a weekly habit mostly happens on.
func weekFavorites(in Input) (Tip, bool) {
	if !in.weekly() {
		return Tip{}, false
	}
	var counts [7]int
	total := 0
	for d := range in.Habit.Done {
		if d > in.Today-weekdayDays && d <= in.Today {
			counts[d.WeekdayIndex()]++
			total++
		}
	}
	if total < favoriteMin {
		return Tip{}, false
	}
	order := []int{0, 1, 2, 3, 4, 5, 6}
	slices.SortStableFunc(order, func(a, b int) int { return counts[b] - counts[a] })
	var names []string
	for _, day := range order[:favoriteCount] {
		if counts[day] > 0 {
			names = append(names, weekdays[day])
		}
	}
	return Tip{Icon: "calendar", Tone: toneGold, Title: "Deine Tage: " + strings.Join(names, " und "), Detail: "dort klappt es am häufigsten", Score: 30}, true
}
