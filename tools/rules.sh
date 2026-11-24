#!/bin/sh
# Structure rules a script can check. The judgement calls are in CODING_STANDARDS.md.
fail=0
# rule MESSAGE PATTERN PATH... : a hit in a non-test Go file fails, internal/view may do anything.
rule() {
	message=$1 pattern=$2
	shift 2
	files=$(find "$@" -name "*.go" ! -name "*_test.go" ! -path "internal/view/*")
	hits=$(grep -nHE "$pattern" $files)
	if [ $? -gt 1 ]; then
		echo "rules.sh: grep failed on $pattern" >&2
		exit 2
	fi
	if [ -n "$hits" ]; then
		printf '%s\n%s\n' "$message" "$hits" >&2
		fail=1
	fi
}

rule "Weekday names come from view.Weekdays:" \
	'"Mo", ?"Di", ?"Mi"' cmd internal
rule "Units are recognised by view.UnitOf, not by hand:" \
	'strings\.(ToLower|EqualFold)\([^)]*\.Unit[^a-zA-Z]' cmd internal
rule "Rule tables change through Service.ChangeRules or ChangeLevels, which reload the rules:" \
	'\.LoadRules\(r\.Context' internal/http
rule "Habits change through Service.ChangeHabits, which settles quest and badges:" \
	'Store\.Q\([^)]*\)\.(SaveHabit|SetHabitArchived)\(' internal/http
exit $fail
