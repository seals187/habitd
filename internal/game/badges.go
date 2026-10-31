package game

import "fmt"

// Category groups badges and defines their colour.
type Category struct {
	Key   string
	Name  string
	Color string
	Sort  int
}

// Categories in display order.
var Categories = []Category{
	{Key: "serien", Name: "Serien", Color: "#F2994A"},
	{Key: "meilensteine", Name: "Meilensteine", Color: "#4FB283"},
	{Key: "vermeiden", Name: "Vermeiden", Color: "#5FD3C6"},
	{Key: "comeback", Name: "Comeback & Joker", Color: "#C79BF2"},
	{Key: "zeit", Name: "Zeit", Color: "#7FB8F0"},
	{Key: "bereiche", Name: "Bereiche", Color: "#F28BB0"},
	{Key: "gute-taten", Name: "Gute Taten", Color: "#E8A0E8"},
	{Key: "familie", Name: "Familie", Color: "#F2B880"},
	{Key: "freunde", Name: "Freunde", Color: "#F5A3A3"},
	{Key: "arbeit", Name: "Arbeit", Color: "#9FB4C8"},
	{Key: "digital", Name: "Digital", Color: "#8C8CF5"},
	{Key: "geld", Name: "Geld", Color: "#D9B26B"},
	{Key: "umwelt", Name: "Umwelt", Color: "#8ACB7A"},
	{Key: "goenn-dir", Name: "Gönn dir", Color: "#F2C14E"},
	{Key: "level", Name: "Level", Color: "#4FB283"},
	{Key: "geheim", Name: "Geheim", Color: "#A3A69F"},
}

// Rule types of a badge.
const (
	RuleStreak  = "streak"
	RuleCount   = "count"
	RuleAmount  = "amount"
	RuleSpecial = "special"
)

// What a rule looks at.
const (
	ScopeAnyHabit  = "any_habit"
	ScopeHabitTag  = "habit_tag"
	ScopeArea      = "area"
	ScopeKindBuild = "kind_build"
	ScopeKindAvoid = "kind_avoid"
	ScopeGlobal    = "global"
)

// Badge is one rule: it is unlocked once the Metric reaches Target.
type Badge struct {
	Key         string
	Name        string
	Hint        string // short label under the medal
	Description string // full explanation, see Rule
	Category    string
	Icon        string
	Color       string // empty uses the colour of the category
	Target      int
	Metric      Metric
	Secret      bool
	Coins       int
	Jokers      int
	Sort        int
	Active      bool
	Builtin     bool
}

// Metric describes what a badge measures. Everything that does not fit the
// building blocks is special: its logic stays in the code, under the badge key.
type Metric struct {
	Rule    string
	Scope   string
	Value   string
	Jokers  bool
	special func(*Facts) int
}

func field(get func(*Facts) int) Metric {
	return Metric{Rule: RuleSpecial, Scope: ScopeGlobal, special: get}
}

func flag(get func(*Facts) bool) Metric {
	return field(func(f *Facts) int {
		if get(f) {
			return 1
		}
		return 0
	})
}

func areaCount(area string) Metric {
	return Metric{Rule: RuleCount, Scope: ScopeArea, Value: area}
}

func areaStreak(area string) Metric {
	return Metric{Rule: RuleStreak, Scope: ScopeArea, Value: area, Jokers: true}
}

func tagCount(tag string) Metric {
	return Metric{Rule: RuleCount, Scope: ScopeHabitTag, Value: tag}
}

func tagStreak(tag string) Metric {
	return Metric{Rule: RuleStreak, Scope: ScopeHabitTag, Value: tag, Jokers: true}
}

func areaSpan(area string) Metric { return field(func(f *Facts) int { return f.AreaSpan[area] }) }

func tagDays(tags ...string) Metric {
	return field(func(f *Facts) int {
		sum := 0
		for _, t := range tags {
			sum += f.TagDays[t]
		}
		return sum
	})
}
func areaDays(area string) Metric { return field(func(f *Facts) int { return f.AreaDays[area] }) }
func avoidStreak(area string) Metric {
	return field(func(f *Facts) int { return f.AvoidAreaStreak[area] })
}

var (
	streak     = Metric{Rule: RuleStreak, Scope: ScopeKindBuild}
	checkins   = Metric{Rule: RuleCount, Scope: ScopeAnyHabit}
	perfect    = field(func(f *Facts) int { return f.PerfectRun })
	clean      = Metric{Rule: RuleStreak, Scope: ScopeKindAvoid, Jokers: true}
	redeemed   = field(func(f *Facts) int { return f.Redeemed })
	reachLevel = field(func(f *Facts) int { return f.Level })
)

// Badges lists all 96 badges with their start values.
var Badges = []Badge{
	{Key: "funke", Name: "Funke", Hint: "3 Tage", Category: "serien", Icon: "flame", Target: 3, Metric: streak},
	{Key: "erste-woche", Name: "Erste Woche", Hint: "7 Tage", Category: "serien", Icon: "flame", Target: 7, Metric: streak},
	{Key: "zwei-wochen", Name: "Zwei Wochen", Hint: "14 Tage", Category: "serien", Icon: "loader", Target: 14, Metric: streak},
	{Key: "monatsflamme", Name: "Monatsflamme", Hint: "30 Tage", Category: "serien", Icon: "calendar", Target: 30, Metric: streak},
	{Key: "durchstarter", Name: "Durchstarter", Hint: "60 Tage", Category: "serien", Icon: "rocket", Target: 60, Metric: streak},
	{Key: "hundert", Name: "Hundert", Hint: "100 Tage", Category: "serien", Icon: "mountain", Target: 100, Metric: streak},
	{Key: "halbes-jahr", Name: "Halbes Jahr", Hint: "180 Tage", Category: "serien", Icon: "mountain", Target: 180, Metric: streak},
	{Key: "ein-jahr", Name: "Ein Jahr", Hint: "365 Tage", Category: "serien", Icon: "trophy", Target: 365, Metric: streak},
	{Key: "unaufhaltsam", Name: "Unaufhaltsam", Hint: "500 Tage", Category: "serien", Icon: "crown", Target: 500, Metric: streak},
	{Key: "legende", Name: "Legende", Hint: "1000 Tage", Category: "serien", Icon: "infinity", Target: 1000, Metric: streak},

	{Key: "erster-schritt", Name: "Erster Schritt", Hint: "1. Check-in", Category: "meilensteine", Icon: "footprints", Target: 1, Metric: checkins},
	{Key: "zehner", Name: "Zehner", Hint: "10 Check-ins", Category: "meilensteine", Icon: "circle-check", Target: 10, Metric: checkins},
	{Key: "fuenfziger", Name: "Fünfziger", Hint: "50 Check-ins", Category: "meilensteine", Icon: "circle-check", Target: 50, Metric: checkins},
	{Key: "hunderter", Name: "Hunderter", Hint: "100 Check-ins", Category: "meilensteine", Icon: "target", Target: 100, Metric: checkins},
	{Key: "viertel-tausend", Name: "Viertel-Tausend", Hint: "250 Check-ins", Category: "meilensteine", Icon: "target", Target: 250, Metric: checkins},
	{Key: "fuenfhundert", Name: "Fünfhundert", Hint: "500 Check-ins", Category: "meilensteine", Icon: "layers", Target: 500, Metric: checkins},
	{Key: "tausend", Name: "Tausend", Hint: "1000 Check-ins", Category: "meilensteine", Icon: "trophy", Target: 1000, Metric: checkins},
	{Key: "erste-belohnung", Name: "Erste Belohnung", Hint: "eingelöst", Category: "meilensteine", Icon: "gift", Target: 1, Metric: redeemed},
	{Key: "jongleur", Name: "Jongleur", Hint: "5 Habits aktiv", Category: "meilensteine", Icon: "layers", Target: 5, Metric: field(func(f *Facts) int { return f.ActiveHabits })},
	{Key: "perfekte-woche", Name: "Perfekte Woche", Hint: "alles 7 Tage", Category: "meilensteine", Icon: "star", Target: 7, Metric: perfect},
	{Key: "perfekter-monat", Name: "Perfekter Monat", Hint: "alles 30 Tage", Category: "meilensteine", Icon: "crown", Target: 30, Metric: perfect},
	{Key: "questmeister", Name: "Questmeister", Hint: "30 Quests in Folge", Category: "meilensteine", Icon: "target", Target: 30, Metric: field(func(f *Facts) int { return f.QuestStreak })},

	{Key: "standhaft", Name: "Standhaft", Hint: "7 Tage clean", Category: "vermeiden", Icon: "shield-check", Target: 7, Metric: clean},
	{Key: "fest", Name: "Fest", Hint: "30 Tage clean", Category: "vermeiden", Icon: "shield-check", Target: 30, Metric: clean},
	{Key: "eisern", Name: "Eisern", Hint: "90 Tage clean", Category: "vermeiden", Icon: "shield-alert", Target: 90, Metric: clean},
	{Key: "befreit", Name: "Befreit", Hint: "1 Jahr clean", Category: "vermeiden", Icon: "crown", Target: 365, Metric: clean},
	{Key: "doppelschutz", Name: "Doppelschutz", Hint: "2× 30 Tage", Category: "vermeiden", Icon: "shield-alert", Target: 2, Metric: field(func(f *Facts) int { return f.AvoidsAt30 })},

	{Key: "comeback", Name: "Comeback", Hint: "14 Tage n. Rückfall", Category: "comeback", Icon: "refresh-cw", Target: 14, Metric: field(func(f *Facts) int { return f.ComebackDays })},
	{Key: "phoenix", Name: "Phönix", Hint: "neuer Rekord", Category: "comeback", Icon: "flame", Target: 1, Metric: flag(func(f *Facts) bool { return f.Phoenix })},
	{Key: "ohne-netz", Name: "Ohne Netz", Hint: "90 T. ohne Joker", Category: "comeback", Icon: "shield-alert", Target: 90, Metric: field(func(f *Facts) int { return f.DaysNoJoker })},
	{Key: "joker-sparer", Name: "Joker-Sparer", Hint: "3 Joker", Category: "comeback", Icon: "vault", Target: MaxJokers, Metric: field(func(f *Facts) int { return f.Jokers })},

	{Key: "fruehaufsteher", Name: "Frühaufsteher", Hint: "7× vor 7 Uhr", Category: "zeit", Icon: "sun", Target: 7, Metric: field(func(f *Facts) int { return f.EarlyCheckins })},
	{Key: "nachteule", Name: "Nachteule", Hint: "7× nach 22 Uhr", Category: "zeit", Icon: "moon", Target: 7, Metric: field(func(f *Facts) int { return f.LateCheckins })},
	{Key: "wochenendheld", Name: "Wochenendheld", Hint: "10 Wochenenden", Category: "zeit", Icon: "wallet", Target: 10, Metric: field(func(f *Facts) int { return f.Weekends })},
	{Key: "feiertagsheld", Name: "Feiertagsheld", Hint: "an Feiertag", Category: "zeit", Icon: "star", Target: 1, Metric: field(func(f *Facts) int { return f.HolidayCheckins })},
	{Key: "ganzes-jahr", Name: "Ganzes Jahr", Hint: "12 Monate aktiv", Category: "zeit", Icon: "calendar", Target: 12, Metric: field(func(f *Facts) int { return f.MonthsActive })},
	{Key: "jahreszeiten", Name: "Jahreszeiten", Hint: "4 Saisons ≥ 80 %", Category: "zeit", Icon: "flower-2", Target: 4, Metric: field(func(f *Facts) int { return f.Seasons })},

	{Key: "buecherwurm", Name: "Bücherwurm", Hint: "20× Lernen", Category: "bereiche", Icon: "book", Target: 20, Metric: areaCount("lernen")},
	{Key: "leseratte", Name: "Leseratte", Hint: "100× Lernen", Category: "bereiche", Icon: "library", Target: 100, Metric: areaCount("lernen")},
	{Key: "eiserner-wille", Name: "Eiserner Wille", Hint: "10 Wo. Sport", Category: "bereiche", Icon: "dumb", Target: 70, Metric: tagStreak("sport")},
	{Key: "marathon", Name: "Marathon", Hint: "100× Sport", Category: "bereiche", Icon: "run", Target: 100, Metric: tagCount("sport")},
	{Key: "ruhepol", Name: "Ruhepol", Hint: "30× Achtsamkeit", Category: "bereiche", Icon: "lotus", Target: 30, Metric: areaCount("achtsamkeit")},
	{Key: "zen-meister", Name: "Zen-Meister", Hint: "100× Achtsamkeit", Category: "bereiche", Icon: "leaf", Target: 100, Metric: areaCount("achtsamkeit")},
	{Key: "hydriert", Name: "Hydriert", Hint: "30 T. Wasser", Category: "bereiche", Icon: "droplet", Target: 30, Metric: tagStreak("wasser")},
	{Key: "gesund-leben", Name: "Gesund leben", Hint: "3 Habits 30 T.", Category: "bereiche", Icon: "heart", Target: 3, Metric: field(func(f *Facts) int { return f.HealthAt30 })},

	{Key: "laecheln", Name: "Lächeln verschenkt", Hint: "10× nett", Category: "gute-taten", Icon: "smile", Target: 10, Metric: areaCount("gute-taten")},
	{Key: "helfende-hand", Name: "Helfende Hand", Hint: "20 gute Taten", Category: "gute-taten", Icon: "hand-heart", Target: 20, Metric: areaCount("gute-taten")},
	{Key: "weltverbesserer", Name: "Weltverbesserer", Hint: "50 gute Taten", Category: "gute-taten", Icon: "sprout", Target: 50, Metric: areaCount("gute-taten")},
	{Key: "ehrenamt", Name: "Ehrenamt", Hint: "10× engagiert", Category: "gute-taten", Icon: "heart-handshake", Target: 10, Metric: areaCount("gute-taten")},
	{Key: "herzensmensch", Name: "Herzensmensch", Hint: "30 Tage Serie", Category: "gute-taten", Icon: "sunrise", Target: 30, Metric: areaStreak("gute-taten")},

	{Key: "angerufen", Name: "Angerufen", Hint: "10× gemeldet", Category: "familie", Icon: "phone", Target: 10, Metric: areaCount("familie")},
	{Key: "familientisch", Name: "Familientisch", Hint: "20 gem. Essen", Category: "familie", Icon: "utensils", Target: 20, Metric: areaCount("familie")},
	{Key: "daheim", Name: "Daheim", Hint: "4 Wochen Serie", Category: "familie", Icon: "house", Target: 28, Metric: areaStreak("familie")},
	{Key: "nicht-vergessen", Name: "Nicht vergessen", Hint: "5 Geburtstage", Category: "familie", Icon: "cake", Target: 5, Metric: areaCount("familie")},
	{Key: "familienmensch", Name: "Familienmensch", Hint: "1 Jahr aktiv", Category: "familie", Icon: "users", Target: 365, Metric: areaSpan("familie")},

	{Key: "gemeldet", Name: "Gemeldet", Hint: "10 Nachrichten", Category: "freunde", Icon: "message-square", Target: 10, Metric: areaCount("freunde")},
	{Key: "treffpunkt", Name: "Treffpunkt", Hint: "10 Treffen", Category: "freunde", Icon: "users", Target: 10, Metric: areaCount("freunde")},
	{Key: "verlaesslich", Name: "Verlässlich", Hint: "20 Zusagen", Category: "freunde", Icon: "handshake", Target: 20, Metric: areaCount("freunde")},
	{Key: "ueberraschung", Name: "Überraschung", Hint: "5 Geschenke", Category: "freunde", Icon: "gift", Target: 5, Metric: areaCount("freunde")},
	{Key: "beste-freunde", Name: "Beste Freunde", Hint: "1 Jahr Kontakt", Category: "freunde", Icon: "heart", Target: 365, Metric: areaSpan("freunde")},

	{Key: "ideengeber", Name: "Ideengeber", Hint: "10 Ideen", Category: "arbeit", Icon: "lightbulb", Target: 10, Metric: areaCount("arbeit")},
	{Key: "initiative", Name: "Initiative", Hint: "5 Projekte", Category: "arbeit", Icon: "rocket", Target: 5, Metric: areaCount("arbeit")},
	{Key: "vorangehen", Name: "Vorangehen", Hint: "Verantwortung", Category: "arbeit", Icon: "flag", Target: 1, Metric: areaCount("arbeit")},
	{Key: "mentor", Name: "Mentor", Hint: "10× Wissen geteilt", Category: "arbeit", Icon: "presentation", Target: 10, Metric: areaCount("arbeit")},
	{Key: "weiterbildung", Name: "Weiterbildung", Hint: "20 Lerneinheiten", Category: "arbeit", Icon: "chart-column", Target: 20, Metric: areaCount("arbeit")},
	{Key: "teamplayer", Name: "Teamplayer", Hint: "20× geholfen", Category: "arbeit", Icon: "handshake", Target: 20, Metric: areaCount("arbeit")},
	{Key: "fokus", Name: "Fokus", Hint: "30 Tage Serie", Category: "arbeit", Icon: "briefcase", Target: 30, Metric: areaStreak("arbeit")},

	{Key: "offline-stunde", Name: "Offline-Stunde", Hint: "7× 1 Std. offline", Category: "digital", Icon: "wifi-off", Target: 7, Metric: areaDays("digital")},
	{Key: "handyfreier-morgen", Name: "Handyfreier Morgen", Hint: "14 Tage", Category: "digital", Icon: "phoneoff", Target: 14, Metric: tagStreak("handy")},
	{Key: "scroll-stopp", Name: "Scroll-Stopp", Hint: "30 T. ohne Doomscrolling", Category: "digital", Icon: "eye", Target: 30, Metric: tagStreak("doomscroll")},
	{Key: "detox-tag", Name: "Detox-Tag", Hint: "ganzer Tag offline", Category: "digital", Icon: "wifi-off", Target: 3, Metric: field(func(f *Facts) int { return f.DigitalClean })},
	{Key: "bildschirm-zen", Name: "Bildschirm-Zen", Hint: "90 Tage", Category: "digital", Icon: "crown", Target: 90, Metric: avoidStreak("digital")},

	{Key: "widerstanden", Name: "Widerstanden", Hint: "7 T. ohne Impulskauf", Category: "geld", Icon: "cartx", Target: 7, Metric: tagStreak("impulskauf")},
	{Key: "budget-profi", Name: "Budget-Profi", Hint: "30 T. im Budget", Category: "geld", Icon: "wallet", Target: 30, Metric: tagStreak("budget")},
	{Key: "selbstgemacht", Name: "Selbstgemacht", Hint: "30 T. ohne Lieferdienst", Category: "geld", Icon: "cup", Target: 30, Metric: tagStreak("lieferdienst")},
	{Key: "sparschwein", Name: "Sparschwein", Hint: "90 T. clean", Category: "geld", Icon: "piggy-bank", Target: 90, Metric: avoidStreak("geld")},
	{Key: "finanzfreiheit", Name: "Finanzfreiheit", Hint: "1 Jahr", Category: "geld", Icon: "gem", Target: 365, Metric: avoidStreak("geld")},

	{Key: "mehrweg", Name: "Mehrweg", Hint: "10× ohne Einweg", Category: "umwelt", Icon: "recycle", Target: 10, Metric: tagDays("einweg")},
	{Key: "velo-statt-auto", Name: "Velo statt Auto", Hint: "20× gefahren", Category: "umwelt", Icon: "bike", Target: 20, Metric: tagDays("auto", "velo")},
	{Key: "veggie-woche", Name: "Veggie-Woche", Hint: "7 T. fleischlos", Category: "umwelt", Icon: "sprout", Target: 7, Metric: tagStreak("fleisch")},
	{Key: "nichts-verschwendet", Name: "Nichts verschwendet", Hint: "30 T.", Category: "umwelt", Icon: "recycle", Target: 30, Metric: tagStreak("verschwendung")},
	{Key: "gruener-fussabdruck", Name: "Grüner Fußabdruck", Hint: "90 T.", Category: "umwelt", Icon: "globe", Target: 90, Metric: areaStreak("umwelt")},

	{Key: "erster-taler", Name: "Erster Taler", Hint: "1. Einlösung", Category: "goenn-dir", Icon: "coin", Target: 1, Metric: redeemed},
	{Key: "sparfuchs", Name: "Sparfuchs", Hint: "1.000 gespart", Category: "goenn-dir", Icon: "piggy-bank", Target: 1000, Metric: field(func(f *Facts) int { return f.Coins })},
	{Key: "geniesser", Name: "Genießer", Hint: "10× eingelöst", Category: "goenn-dir", Icon: "gift", Target: 10, Metric: redeemed},
	{Key: "grosser-wunsch", Name: "Großer Wunsch", Hint: "≥ 500 Taler", Category: "goenn-dir", Icon: "gem", Target: 500, Metric: field(func(f *Facts) int { return f.MaxRedeemPrice })},
	{Key: "schatzmeister", Name: "Schatzmeister", Hint: "10.000 verdient", Category: "goenn-dir", Icon: "vault", Target: 10000, Metric: field(func(f *Facts) int { return f.CoinsEarned })},

	{Key: "aufsteiger", Name: "Aufsteiger", Hint: "Level 5", Category: "level", Icon: "arrow-big-up", Target: 5, Metric: reachLevel},
	{Key: "erfahren", Name: "Erfahren", Hint: "Level 10", Category: "level", Icon: "arrow-big-up", Target: 10, Metric: reachLevel},
	{Key: "profi", Name: "Profi", Hint: "Level 25", Category: "level", Icon: "star", Target: 25, Metric: reachLevel},
	{Key: "meister", Name: "Meister", Hint: "Level 50", Category: "level", Icon: "crown", Target: 50, Metric: reachLevel},

	{Key: "mitternacht", Name: "Mitternacht", Hint: "Check-in 00:00–00:05", Category: "geheim", Icon: "moon", Target: 1, Secret: true, Metric: flag(func(f *Facts) bool { return f.Midnight })},
	{Key: "pi-tag", Name: "Pi-Tag", Hint: "am 14.3.", Category: "geheim", Icon: "circle-check", Target: 1, Secret: true, Metric: flag(func(f *Facts) bool { return f.PiDay })},
	{Key: "schnapszahl", Name: "Schnapszahl", Hint: "Serie 111 oder 222", Category: "geheim", Icon: "sparkles", Target: 1, Secret: true, Metric: flag(func(f *Facts) bool { return f.Palindrome })},
	{Key: "ein-jahr-dabei", Name: "1 Jahr dabei", Hint: "365 Tage dabei", Category: "geheim", Icon: "cake", Target: 365, Secret: true, Metric: field(func(f *Facts) int { return f.DaysInstalled })},
	{Key: "blitzstart", Name: "Blitzstart", Hint: "alles vor 8 Uhr", Category: "geheim", Icon: "zap", Target: 1, Secret: true, Metric: flag(func(f *Facts) bool { return f.Blitz })},
}

// current reads the progress of a badge from the facts.
func (b Badge) current(f *Facts) int {
	m := b.Metric
	switch m.Rule {
	case RuleStreak:
		return f.bestStreak(m)
	case RuleCount:
		return f.count(m)
	case RuleAmount:
		return f.amount(m)
	}
	if m.special == nil {
		return 0
	}
	return m.special(f)
}

// Value is the progress of a badge, clamped to its target.
func (b Badge) Value(f *Facts) int { return min(b.current(f), b.Target) }

// Reached reports whether the rule is fulfilled.
func (b Badge) Reached(f *Facts) bool { return b.current(f) >= b.Target }

var scopeLabels = map[string]string{ScopeAnyHabit: "habit: any", ScopeHabitTag: "tag", ScopeArea: "area",
	ScopeKindBuild: "habit: build", ScopeKindAvoid: "habit: avoid", ScopeGlobal: "global"}

// Formula is the rule in short form, like streak(habit: any) >= 60.
func (b Badge) Formula() string {
	m := b.Metric
	if m.Rule == RuleSpecial {
		return fmt.Sprintf("special(%s) >= %d", b.Key, b.Target)
	}
	scope := scopeLabels[m.Scope]
	if m.Value != "" {
		scope += ": " + m.Value
	}
	return fmt.Sprintf("%s(%s) >= %d", m.Rule, scope, b.Target)
}

// Condition is the rule in plain words for the admin list.
func (b Badge) Condition(areaName func(string) string) string {
	m := b.Metric
	subject := ""
	switch m.Scope {
	case ScopeHabitTag:
		subject = "Tag " + m.Value + " · "
	case ScopeArea:
		subject = "Bereich " + areaName(m.Value) + " · "
	}
	switch m.Rule {
	case RuleStreak:
		if m.Scope == ScopeKindAvoid {
			return fmt.Sprintf("Vermeiden-Serie ≥ %d", b.Target)
		}
		return fmt.Sprintf("%sSerie ≥ %d Tage", subject, b.Target)
	case RuleCount:
		if subject == "" {
			return fmt.Sprintf("Check-ins ≥ %d", b.Target)
		}
		return fmt.Sprintf("%sAnzahl ≥ %d", subject, b.Target)
	case RuleAmount:
		return fmt.Sprintf("%sMenge ≥ %d", subject, b.Target)
	}
	return b.Hint
}

// BadgeByKey finds a default badge.
func BadgeByKey(key string) (Badge, bool) { return defaults.BadgeByKey(key) }

// CategoryByKey finds a default category.
func CategoryByKey(key string) (Category, bool) { return defaults.CategoryByKey(key) }

// NewlyReached returns the default rules that are fulfilled but not unlocked yet.
func NewlyReached(f *Facts, unlocked map[string]bool) []Badge {
	return defaults.NewlyReached(f, unlocked)
}
