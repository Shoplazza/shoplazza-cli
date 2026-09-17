package output

// pendingNotice is the agent-facing "_notice" block merged into the top level
// of json success envelopes (never pretty/table/ndjson — those aren't the
// machine envelope). It is set once, before command execution, and read-only
// thereafter, so no locking is needed.
var pendingNotice map[string]any

// SetNotice registers the _notice block emitted in json success envelopes.
// A nil or empty map clears it. Call once, before command execution.
func SetNotice(n map[string]any) {
	if len(n) == 0 {
		pendingNotice = nil
		return
	}
	pendingNotice = n
}
