# Changelog

All notable changes to habitd are listed here, newest first. Every release has its entry before it is tagged.

To update a running installation, back up `data/` and run:

```sh
docker compose pull
docker compose up -d
```

Migrations run automatically on start and create a backup of the database next to it first (`*.bak-<timestamp>`).

## [0.0.7] - 2026-10-31

### Added

- **Groups on Today:** habits are split into Offen and Erreicht, each with its count and a button to fold or open all of them. Vermeiden has the same head.
- **Goal bars:** every card shows one bar per unit of its goal in the colour of its area, such as six bars for 3 l in steps of 0,5. Weekly and monthly rhythms get one bar per check-in.
- **Beyond the goal:** more than the goal shows in gold. The ring runs on in a gold lap around a dark centre, gold bars follow a white mark, and the value such as `3,5/3 l` or `40/20 min` stands beside the week count. This is display only, XP and coins are still paid once.
- **Reached look:** a reached card has a green ring, a green add button and a green glow that fades from the ring to the right.

### Changed

- Folded and opened cards are remembered per device instead of for all devices. Cards folded by hand open once after the update.
- The level card has a new look with a night sky, lavender bars and the day's XP in blue.
- Weekdays are always short, such as Sa or So, in dates, quest texts, the week strip and the month calendar.
- `−` on a weekly goal also works on days without an entry of their own and takes back the last click of the week.
- Day blocks on reached cards have more contrast.

## [0.0.6] - 2026-10-31

### Added

- **Habit size:** `−` and `+` beside the first habit heading on Today make the habit cards smaller or larger, from 80 % to 120 %. Text, rings, week strip and buttons scale with the card, and each device keeps its own size.

## [0.0.5] - 2026-10-31

### Added

- **Quest habits:** under Profile → Quest-Habits, or from the quest page, you choose which habits count for the quests over all habits, such as Wochenend-Held and Alles erledigt. The choice is per user and per quest, applies to today's quest right away and is part of the export.
- **Open reached habits:** a reached card still folds by itself, and its arrow now opens it for the rest of the day.

### Changed

- Open habit cards take a little less height, so more of them fit on a phone screen.
- Wochenend-Held is no longer drawn when no habit counts for it and "Nur passende Quests" is on.

### Fixed

- The app showed version 0.0.3 in the 0.0.4 image. The version now comes from the git tag at build time.
- The tap area of the fold arrow no longer covers the top edge of the `+1` button.

## [0.0.4] - 2026-10-31

### Added

- **Weekly goals:** counters and amounts can count per week instead of per day. Entries add up over the week, and XP, coins and the streak are earned once when the goal is reached. Going back below the goal takes them back.
- **Amounts in the statistics:** habits with a unit now show the total for the week, month or year, a ring for the share of the period goal and a comparison with the period before. The week has bars with values, the month shows the value in each calendar day, and the year shows the sum per month. Everything is calculated from the entries.
- **Readable units:** minutes from 60 on become hours, steps become `6,4k` or `1,3 Mio.`, and the year view puts totals into everyday terms such as bathtubs or marathons.
- **Fold habits:** any card on Today can be folded to one line. The ring keeps showing the progress, and a small week strip shows how the week is going.
- **More rhythms:** `1× / Woche` and `2× / Woche`.
- **Heute frei:** habits with fixed weekdays stay visible on their free days, with the next planned day.

### Changed

- Reached habits fold by themselves. A met weekly or monthly goal stays folded until the period ends.
- Adding a habit from a template opens the form first, so you can adjust it before saving. The onboarding still adds several at once.
- The template "Eine neue Idee einbringen" is now once a week.
- The habit statistics fit on one phone screen without scrolling.

### Known issue

- The app reports version 0.0.3 in this release. Fixed in 0.0.5.

## [0.0.3] - 2026-10-31

### Added

- **Several users:** roles, sessions, invite links, private data per user, and emergency admin commands on the command line.
- **Admin area:** levels with an XP curve, badges with categories and import/export, habit areas, templates, coins and jokers, users and system.
- **Daily quests:** a configurable pool, a weekly plan, events, recurring rules, a quest page in the app, a 7-day joker bonus and the new Questmeister badge (96 badges in total).
- **Counter habits:** plus and minus on the Today card, with freely chosen decimal steps such as 0.25.
- **Redesigned screens:** Today with a week strip and habit cards, habit statistics, and the reward shop.
- **Badge switch:** badges can be turned off for the whole instance, which also removes the tab.
- **Account actions:** delete single habits or all of them, reset everything, and log out from the profile.

### Changed

- Questions before risky actions use the app's own dialog instead of the browser's.
- Resetting everything also removes rewards, so a fresh start has an empty reward list.
- Badge categories wrap on desktop, so all of them can be reached without a touch screen.
- Page changes are faster and no longer shake or visibly reload.
- The README screenshots now show the real app.

### Fixed

- A rejected form showed the raw page source in a pop-up. The error now appears on the page and your input stays.
- The level table cut off XP and coin values at common desktop widths.
- A rejected XP threshold could collide with a level hidden in the short table. All levels are now shown after an error.

### Upgrade notes

- This release changes the database a lot, so back up `data/` yourself as well.
- Set `HABITD_ADMIN_USER` and `HABITD_ADMIN_PASSWORD` in `.env` before the first start of a new installation. See the README for all variables.

## [0.0.2] - 2026-10-30

### Added

- A prebuilt Docker image on the GitHub Container Registry, tagged `latest` and with the version number. A workflow publishes it for every version tag.

### Fixed

- `docker-compose.yml` pointed at `habitd:latest`, an image that only existed locally. It now uses `ghcr.io/seals187/habitd:latest`.

## [0.0.1] - 2026-10-30

First release of habitd, a lightweight, self-hosted habit tracker with rewards that runs as a single Docker container.

### Added

- Onboarding and 200 habit templates with search and a library.
- Habits to build or avoid: daily, n times per week or month, or on chosen weekdays.
- XP, levels, streak bonuses, a daily quest, jokers and 95 badges in 16 categories.
- Rewards paid with Taler or unlocked by a streak.
- Week, month and year statistics with heatmap.
- Installable PWA with offline shell and Web Push reminders.
- JSON export and import, plus a small JSON API.

### Known limits

- The UI is in German.
- Web Push needs HTTPS and has not yet been tested end to end against a real push service.

[0.0.7]: https://github.com/seals187/habitd/releases/tag/v0.0.7
[0.0.6]: https://github.com/seals187/habitd/releases/tag/v0.0.6
[0.0.5]: https://github.com/seals187/habitd/releases/tag/v0.0.5
[0.0.4]: https://github.com/seals187/habitd/releases/tag/v0.0.4
[0.0.3]: https://github.com/seals187/habitd/releases/tag/v0.0.3
[0.0.2]: https://github.com/seals187/habitd/releases/tag/v0.0.2
[0.0.1]: https://github.com/seals187/habitd/releases/tag/v0.0.1
