# Coding standards

Read during review, against the diff. `tools/rules.sh` already fails the mechanical cases, so these
are the judgement calls. Terms are defined in `GLOSSARY.md`.

## Where rules live

- **Day and period rules belong in `internal/game`.** How a day went, what it is worth, whether
  today, a week or a month is reached, rates and progress: a method on `HabitData`, with a table
  test in `game`. A handler in `internal/http` maps the result to CSS classes and words. A new
  helper in `internal/http` that loops over `h.Done`, `h.Joker`, `h.Relapses` or `h.Entries` to
  decide something is a copy of a rule: move it down, or use the existing one.
- **One rate.** The fulfilment rate is `Quota` (or `MonthQuota` for bars side by side), progress
  in a period is `Goals`, a plain count is `Achieved`. A page that shows a percentage or "x/y"
  uses one of them, so two pages never disagree about the same habit.
- **Statistics start versus creation.** Statistics of one habit use `ForStatistics()`. Quests,
  badges, rewards and anything that goes back into the World use the habit as loaded.

## Wording

- Amounts, units, streak spans and weekday names are written by `internal/view` (`view.UnitOf`,
  `view.Span`, `view.StreakTag`, `view.Weekdays`). A new unit or a new wording variant is added
  there once and reaches cards, calendar, statistics and tips together.

## Writes

- A change of rule tables goes through `Service.ChangeRules` or `ChangeLevels`, a change of habits
  through `ChangeHabits`, so the cache, the quest and the badges follow in the same transaction.
- A command on the days of one habit runs through `onHabit` and writes through `dayWriter`.
  Every write updates the store and the loaded `HabitData` together; a write that only touches
  one of them leaves the next step of the transaction looking at stale data.

## Tests

- A rule gets its test at the module that owns it: table tests on `HabitData` in `game`, SQLite
  tests in `service`. HTTP tests check that a page shows the result, not the rule itself.
