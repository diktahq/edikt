package adapter

// events.go — edikt's OWN on-disk event-log artifact shapes for
// task-created.go/task-completed.go/worktreecreate.go/worktreeremove.go
// (PLAN-v080-reliability-core Phase 14). These are NOT Claude Code wire
// replies — they are lines appended to $HOME/.edikt/events.jsonl, edikt's
// own ledger format — but two of them (TaskCreatedEventJSON,
// TaskCompletedEventJSON) use the field names "task_id" and "task_name",
// which happen to share their exact spelling with host wire fields already
// on tools/edikt/check/no-claude-leak-in-hook-core.sh's WIRE_FIELDS
// denylist. Coincidence of spelling, not coupling to the host protocol —
// but SP-004's own rule is about the SPELLING, not the protocol it serves
// (see LastSessionSummary's own doc comment above for the established
// precedent), so these live here rather than in a handler file, exactly the
// way session-end.sh's own artifact writer does.

import "strconv"

// TaskCreatedEventJSON mirrors task-created.sh's own python3 json.dumps
// call, field order ts, event, task_id, task_name, phase, plan. phase and
// plan render as JSON null when empty — `X or None` in the bash original —
// matching the bytes the recorded corpus pins exactly.
func TaskCreatedEventJSON(ts, taskID, taskName, phase, plan string) ([]byte, error) {
	type event struct {
		TS       string  `json:"ts"`
		Event    string  `json:"event"`
		TaskID   string  `json:"task_id"`
		TaskName string  `json:"task_name"`
		Phase    *string `json:"phase"`
		Plan     *string `json:"plan"`
	}
	e := event{TS: ts, Event: "task_created", TaskID: taskID, TaskName: taskName}
	if phase != "" {
		e.Phase = &phase
	}
	if plan != "" {
		e.Plan = &plan
	}
	return MarshalPythonCompact(e)
}

// TaskCompletedEventJSON mirrors task-completed.sh's own python3 json.dumps
// call, field order ts, event, task_id, task_name, status.
func TaskCompletedEventJSON(ts, taskID, taskName, status string) ([]byte, error) {
	type event struct {
		TS       string `json:"ts"`
		Event    string `json:"event"`
		TaskID   string `json:"task_id"`
		TaskName string `json:"task_name"`
		Status   string `json:"status"`
	}
	e := event{TS: ts, Event: "task_completed", TaskID: taskID, TaskName: taskName, Status: status}
	return MarshalPythonCompact(e)
}

// WorktreeEventJSON mirrors worktree-create.sh's and worktree-remove.sh's
// own python3 json.dumps calls, field order ts, event, worktree. eventType
// is "worktree_create" or "worktree_remove". Neither "event" nor "worktree"
// is on the WIRE_FIELDS denylist, so this one could live in a handler file
// without tripping the leak gate — kept here anyway, beside its two
// siblings above, so all three of this hook batch's own event-log shapes
// stay in one place.
func WorktreeEventJSON(ts, eventType, worktree string) ([]byte, error) {
	type event struct {
		TS       string `json:"ts"`
		Event    string `json:"event"`
		Worktree string `json:"worktree"`
	}
	return MarshalPythonCompact(event{TS: ts, Event: eventType, Worktree: worktree})
}

// SubagentStartMessage renders subagent-start.sh's own governance heads-up
// text, exactly reproducing its two-shape `if [ -n "$SUBAGENT_TYPE" ]`
// branch. The ".claude/rules/" segment is a host on-disk layout convention
// (RulesDirSegment), so it is interpolated here rather than in the handler
// (SP-004/SAC-003) — the message text around it is edikt's own business
// copy, not host wire vocabulary, but the literal directory spelling still
// belongs to this package.
func SubagentStartMessage(subagentType string, adrCount, invCount int) string {
	if subagentType != "" {
		return "Subagent " + subagentType + " spawned. edikt governance active: " +
			strconv.Itoa(adrCount) + " ADRs, " + strconv.Itoa(invCount) + " invariants. Cite ADR-NNN / " +
			"INV-NNN when a decision applies; defer to compiled rules in " + RulesDirSegment + " over memory."
	}
	return "edikt governance active: " + strconv.Itoa(adrCount) + " ADRs, " + strconv.Itoa(invCount) +
		" invariants. Cite ADR-NNN / INV-NNN when a decision applies."
}
