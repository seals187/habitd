package game

// SecretRule stands in for the rule of a secret badge that is still locked.
const SecretRule = "Dieses Abzeichen ist geheim. Die Regel siehst du, sobald du es freigeschaltet hast."

// Rule explains in full what unlocks the badge. Hint is only the short label.
func (b Badge) Rule() string {
	if b.Description != "" {
		return b.Description
	}
	return badgeRules[b.Key]
}

// badgeRules must describe what the Metric of a badge really measures.
var badgeRules = map[string]string{
	"funke":        "Komme bei einem Aufbau-Habit in der laufenden Serie auf 3 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"erste-woche":  "Komme bei einem Aufbau-Habit in der laufenden Serie auf 7 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"zwei-wochen":  "Komme bei einem Aufbau-Habit in der laufenden Serie auf 14 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"monatsflamme": "Komme bei einem Aufbau-Habit in der laufenden Serie auf 30 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"durchstarter": "Komme bei einem Aufbau-Habit in der laufenden Serie auf 60 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"hundert":      "Komme bei einem Aufbau-Habit in der laufenden Serie auf 100 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"halbes-jahr":  "Komme bei einem Aufbau-Habit in der laufenden Serie auf 180 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"ein-jahr":     "Komme bei einem Aufbau-Habit in der laufenden Serie auf 365 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"unaufhaltsam": "Komme bei einem Aufbau-Habit in der laufenden Serie auf 500 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",
	"legende":      "Komme bei einem Aufbau-Habit in der laufenden Serie auf 1.000 Tage mit Check-in. Joker-Tage halten die Serie, zählen hier aber nicht mit.",

	"erster-schritt":  "Hake zum ersten Mal ein Habit ab.",
	"zehner":          "Sammle insgesamt 10 Check-ins, egal bei welchem Habit.",
	"fuenfziger":      "Sammle insgesamt 50 Check-ins, egal bei welchem Habit.",
	"hunderter":       "Sammle insgesamt 100 Check-ins, egal bei welchem Habit.",
	"viertel-tausend": "Sammle insgesamt 250 Check-ins, egal bei welchem Habit.",
	"fuenfhundert":    "Sammle insgesamt 500 Check-ins, egal bei welchem Habit.",
	"tausend":         "Sammle insgesamt 1.000 Check-ins, egal bei welchem Habit.",
	"erste-belohnung": "Löse in „Gönn dir“ deine erste Belohnung ein.",
	"jongleur":        "Habe gleichzeitig 5 aktive Habits. Archivierte zählen nicht.",
	"perfekte-woche":  "Erledige an 7 Tagen in Folge alle geplanten Habits, ohne Rückfall und mit erreichten Wochenzielen. Du brauchst dafür mindestens 2 Habits.",
	"perfekter-monat": "Erledige an 30 Tagen in Folge alle geplanten Habits, ohne Rückfall und mit erreichten Wochenzielen. Du brauchst dafür mindestens 2 Habits.",
	"questmeister":    "Schaffe 30 Tage in Folge deine Tagesquest.",

	"standhaft":    "Bleibe bei einem Vermeiden-Habit 7 Tage am Stück ohne Rückfall.",
	"fest":         "Bleibe bei einem Vermeiden-Habit 30 Tage am Stück ohne Rückfall.",
	"eisern":       "Bleibe bei einem Vermeiden-Habit 90 Tage am Stück ohne Rückfall.",
	"befreit":      "Bleibe bei einem Vermeiden-Habit 365 Tage am Stück ohne Rückfall.",
	"doppelschutz": "Halte bei 2 Vermeiden-Habits gleichzeitig eine Serie von mindestens 30 Tagen.",

	"comeback":     "Hattest du bei einem Vermeiden-Habit einen Rückfall? Dann bleibe danach 14 Tage am Stück ohne Rückfall.",
	"phoenix":      "Übertriff nach einem Rückfall mit der laufenden Serie deinen bisherigen Rekord bei diesem Vermeiden-Habit.",
	"ohne-netz":    "Komme 90 Tage ohne Joker aus. Jeder eingesetzte Joker startet die Zählung neu.",
	"joker-sparer": "Besitze gleichzeitig 3 Joker, das ist der Höchststand.",

	"fruehaufsteher": "Hake 7-mal ein Habit vor 7 Uhr ab. Nachgetragene Tage zählen nicht.",
	"nachteule":      "Hake 7-mal ein Habit ab 22 Uhr ab. Nachgetragene Tage zählen nicht.",
	"wochenendheld":  "Erledige an 10 Wochenenden am Sa und So alle geplanten Habits, ohne Rückfall.",
	"feiertagsheld":  "Hake an einem Feiertag ein Habit ab.",
	"ganzes-jahr":    "Sei 12 Monate in Folge aktiv: In jedem Monat braucht es mindestens einen Check-in.",
	"jahreszeiten":   "Erreiche in allen 4 Jahreszeiten eine Quote von mindestens 80 %. Es zählen Habits mit festen Tagen, und eine Jahreszeit braucht mindestens 60 geplante Check-ins.",

	"buecherwurm":    "Sammle 20 Check-ins im Bereich Lernen.",
	"leseratte":      "Sammle 100 Check-ins im Bereich Lernen.",
	"eiserner-wille": "Halte bei einem Sport-Habit eine Serie von 10 Wochen, also 70 Tagen.",
	"marathon":       "Sammle 100 Check-ins mit Sport-Habits.",
	"ruhepol":        "Sammle 30 Check-ins im Bereich Achtsamkeit.",
	"zen-meister":    "Sammle 100 Check-ins im Bereich Achtsamkeit.",
	"hydriert":       "Halte bei einem Wasser-Habit eine Serie von 30 Tagen.",
	"gesund-leben":   "Halte bei 3 Habits im Bereich Gesundheit gleichzeitig eine Serie von mindestens 30 Tagen.",

	"laecheln":        "Sammle 10 Check-ins im Bereich Gute Taten.",
	"helfende-hand":   "Sammle 20 Check-ins im Bereich Gute Taten.",
	"weltverbesserer": "Sammle 50 Check-ins im Bereich Gute Taten.",
	"ehrenamt":        "Sammle 10 Check-ins im Bereich Gute Taten.",
	"herzensmensch":   "Halte bei einem Habit im Bereich Gute Taten eine Serie von 30 Tagen.",

	"angerufen":       "Sammle 10 Check-ins im Bereich Familie.",
	"familientisch":   "Sammle 20 Check-ins im Bereich Familie.",
	"daheim":          "Halte bei einem Habit im Bereich Familie eine Serie von 4 Wochen, also 28 Tagen.",
	"nicht-vergessen": "Sammle 5 Check-ins im Bereich Familie.",
	"familienmensch":  "Bleibe bei einem Habit im Bereich Familie dran: Zwischen erstem und letztem Check-in müssen mindestens 365 Tage liegen.",

	"gemeldet":      "Sammle 10 Check-ins im Bereich Freunde.",
	"treffpunkt":    "Sammle 10 Check-ins im Bereich Freunde.",
	"verlaesslich":  "Sammle 20 Check-ins im Bereich Freunde.",
	"ueberraschung": "Sammle 5 Check-ins im Bereich Freunde.",
	"beste-freunde": "Bleibe bei einem Habit im Bereich Freunde dran: Zwischen erstem und letztem Check-in müssen mindestens 365 Tage liegen.",

	"ideengeber":    "Sammle 10 Check-ins im Bereich Arbeit.",
	"initiative":    "Sammle 5 Check-ins im Bereich Arbeit.",
	"vorangehen":    "Sammle deinen ersten Check-in im Bereich Arbeit.",
	"mentor":        "Sammle 10 Check-ins im Bereich Arbeit.",
	"weiterbildung": "Sammle 20 Check-ins im Bereich Arbeit.",
	"teamplayer":    "Sammle 20 Check-ins im Bereich Arbeit.",
	"fokus":         "Halte bei einem Habit im Bereich Arbeit eine Serie von 30 Tagen.",

	"offline-stunde":     "Sammle im Bereich Digital 7 Punkte: Jeder Check-in und jeder Tag ohne Rückfall zählt einen.",
	"handyfreier-morgen": "Halte bei einem Habit für den handyfreien Morgen eine Serie von 14 Tagen.",
	"scroll-stopp":       "Halte bei einem Habit gegen Doomscrolling eine Serie von 30 Tagen.",
	"detox-tag":          "Halte bei 3 Vermeiden-Habits im Bereich Digital gleichzeitig eine laufende Serie.",
	"bildschirm-zen":     "Bleibe bei einem Vermeiden-Habit im Bereich Digital 90 Tage am Stück ohne Rückfall.",

	"widerstanden":   "Halte bei einem Habit gegen Impulskäufe eine Serie von 7 Tagen.",
	"budget-profi":   "Halte bei einem Budget-Habit eine Serie von 30 Tagen.",
	"selbstgemacht":  "Halte bei einem Habit gegen Lieferdienste eine Serie von 30 Tagen.",
	"sparschwein":    "Bleibe bei einem Vermeiden-Habit im Bereich Geld 90 Tage am Stück ohne Rückfall.",
	"finanzfreiheit": "Bleibe bei einem Vermeiden-Habit im Bereich Geld 365 Tage am Stück ohne Rückfall.",

	"mehrweg":             "Sammle mit Habits gegen Einweg 10 Punkte: Jeder Check-in und jeder Tag ohne Rückfall zählt einen.",
	"velo-statt-auto":     "Sammle mit Velo- oder Auto-Habits 20 Punkte: Jeder Check-in und jeder Tag ohne Rückfall zählt einen.",
	"veggie-woche":        "Halte bei einem Habit für fleischlose Tage eine Serie von 7 Tagen.",
	"nichts-verschwendet": "Halte bei einem Habit gegen Verschwendung eine Serie von 30 Tagen.",
	"gruener-fussabdruck": "Halte bei einem Habit im Bereich Umwelt eine Serie von 90 Tagen.",

	"erster-taler":   "Löse in „Gönn dir“ deine erste Belohnung ein.",
	"sparfuchs":      "Habe gleichzeitig 1.000 Taler auf dem Konto.",
	"geniesser":      "Löse in „Gönn dir“ 10 Belohnungen ein.",
	"grosser-wunsch": "Löse eine Belohnung ein, die mindestens 500 Taler kostet.",
	"schatzmeister":  "Verdiene insgesamt 10.000 Taler. Ausgegebene Taler werden nicht abgezogen.",

	"aufsteiger": "Erreiche Level 5.",
	"erfahren":   "Erreiche Level 10.",
	"profi":      "Erreiche Level 25.",
	"meister":    "Erreiche Level 50.",

	"mitternacht":    "Hake ein Habit zwischen 00:00 und 00:05 Uhr ab.",
	"pi-tag":         "Hake am 14. März ein Habit ab.",
	"schnapszahl":    "Erreiche eine Serie von genau 111 oder 222 Tagen.",
	"ein-jahr-dabei": "Nutze habitd seit 365 Tagen.",
	"blitzstart":     "Erledige an einem Tag alle geplanten Habits vor 8 Uhr.",
}
