# habitd

- `docs/DECISIONS.md` records each rule where the spec and the design images disagreed. Read the section on the area before changing a rule or a screen.
- `GLOSSARY.md` names the domain modules: day mark, rate, progress, today's goal, unit, day writer.
- `CODING_STANDARDS.md` holds the review rules; `tools/rules.sh` checks the mechanical ones.
- Checks: `make check`, or `sh tools/check.sh` where Go is missing (it runs in a Go container). `sh tools/check.sh go test ./internal/game` runs a single command there.
