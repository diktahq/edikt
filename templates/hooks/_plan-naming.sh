#!/usr/bin/env bash
# THE ONE DEFINITION of how a plan's state files are named.
#
# WHY THIS FILE EXISTS
#
# `commands/sdlc/plan.md:347` interpolates `${PLAN_SLUG}` into
# `phase-start-${PLAN_SLUG}-${PHASE_NUM}.sha` and NOTHING ANYWHERE DEFINES IT.
# The convention existed only as whatever the last writer happened to do, and
# three independent forms had already appeared across hooks, commands and docs.
#
# The cost was measured, not hypothetical: a check written to detect phases
# skipping the harness looked for `phase-start-PLAN-<id>-N.sha`, the file on
# disk is `phase-start-<id>-N.sha`, and the check reported EVERY plan in the
# repository as hand-driven on its first run. A gate that condemns a user's
# whole history on first run is disabled within a minute, and the real signal
# inside it dies with it.
#
# TWO NAMES, DELIBERATELY, AND THEY ARE NOT INTERCHANGEABLE
#
# The two state families on disk genuinely disagree today:
#
#   .edikt/state/plan-eval/PLAN-<id>-eval.json     <- KEEPS the PLAN- prefix
#   .edikt/state/phase-start-<id>-<n>.sha          <- DROPS it
#
# AMBIGUITY RECORDED AND DATED (2026-08-13), not tolerated as if designed: this
# split is a compatibility fact about state already on disk, not an intention.
# Both forms are defined here so every consumer reads the same answer instead of
# guessing; unifying them requires migrating existing state and is not this
# release's work. Until then, a consumer that needs to match EITHER family must
# use the accessor for that family — never derive one from the other.

# edikt_plan_stem <path-to-plan.md>
#   The file stem: basename minus `.md`. KEEPS a leading `PLAN-`.
#   Used by: plan-eval state.
edikt_plan_stem() {
	local b
	b=$(basename "$1")
	printf '%s' "${b%.md}"
}

# edikt_plan_id <path-to-plan.md>
#   The plan ID: the stem with a leading `PLAN-` removed.
#   Used by: phase-start SHA files, post-flight reports.
edikt_plan_id() {
	local stem
	stem=$(edikt_plan_stem "$1")
	printf '%s' "${stem#PLAN-}"
}

# edikt_phase_start_sha <plan-path> <phase-number>
#   The canonical path of a phase-start SHA file, relative to the project root.
edikt_phase_start_sha() {
	printf '.edikt/state/phase-start-%s-%s.sha' "$(edikt_plan_id "$1")" "$2"
}

# edikt_plan_eval_state <plan-path>
#   The canonical path of a plan's evaluation state file.
edikt_plan_eval_state() {
	printf '.edikt/state/plan-eval/%s-eval.json' "$(edikt_plan_stem "$1")"
}
