# Glossar

Begriffe, mit denen Code und Entscheidungen arbeiten. Der Name im Code steht in Klammern.

- **Statistik-Start** (`HabitData.ForStatistics`): Die Statistik eines Habits beginnt am ersten Tag
  mit irgendetwas darauf, auch vor dem Anlegen. Quests, Abzeichen und Belohnungen rechnen weiter ab
  dem Anlegen (`Start`). Die Kopie für die Statistik kommt nie zurück in eine World.
- **Tagesmarke** (`game.DayMark`, `HabitData.Mark`): wie ein Tag eines Habits gelaufen ist, aus Sicht
  von heute: frei, vor dem Start, Zukunft, offen, erreicht, teilweise, Joker oder verpasst. Woche,
  Monat, Jahr und die Wochenleiste auf „Heute“ übersetzen sie nur in ihre CSS-Klassen.
- **Tageswert** (`HabitData.Value`): was ein Tag in den Diagrammen wert ist.
- **Erfüllungsquote** (`Quota`): erreichte gegen geplante Ziele, gleich auf Mein Level und der
  Habit-Seite. Offene Tage und laufende Wochen zählen erst, wenn sie erreicht oder vorbei sind, ein
  Joker zählt als erreicht, mehr als das Ziel zählt nicht. Eine Woche gehört zum Zeitraum ihres
  Sonntags. `MonthQuota` ist die Quote eines Monatsbalkens, ohne dass eine Woche doppelt zählt.
- **Fortschritt** (`Goals`): die bisher erreichten Ziele eines Zeitraums gegen alle seine Ziele,
  auch die noch kommenden, etwa „2/3“ am Mittwoch. Er steht in der Wochenansicht und ist die
  Grundlage der Mengenziele.
- **Erledigte Tage** (`Achieved`): eine reine Zahl der Tage mit Check-in, keine Quote.
- **Tagesziel von heute** (`game.TodayGoal`): wie weit ein Habit heute ist, mit Gruppe (offen,
  erreicht, vermeiden, heute frei), Füllstand und Einheiten für die Balken. Der mittlere Füllstand
  (`game.DayFill`) wählt die Stufe der Figur.
- **Einheit** (`view.Unit`): erkennt die Einheit eines Habits und schreibt Mengen in den
  Varianten Label, Bare, Tag, Over und Signed.
- **Regeländerung** (`Service.ChangeRules`, `ChangeLevels`): ändert Abzeichen, Kategorien, Quests
  oder Levels und lädt die Regeln in derselben Transaktion neu.
- **Habit-Änderung** (`Service.ChangeHabits`): ändert Habits und rechnet Quest und Abzeichen in
  derselben Transaktion nach.
- **Tagesschreiber** (`service.dayWriter`): schreibt die Tage eines Habits so, dass Datenbank und
  die geladene World gleich bleiben. Die Befehle wie Abhaken oder Nachtragen laufen über
  `onHabit`: laden, prüfen, schreiben, nachrechnen.
