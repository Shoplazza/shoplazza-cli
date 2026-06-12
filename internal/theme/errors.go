// Package theme provides error-classification helpers shared by the
// theme subpackages and the cmd/themes/* shortcut layer. Every helper
// maps to exactly one of the five envelope types
// (api/validation/auth/network/internal).
package theme

import (
	"fmt"
	"math"
	"time"

	"shoplazza-cli-v2/internal/output"
)

// ErrAuthExpired flags an OAuth / 401 / keychain-miss path and suggests
// `shoplazza auth login`.
func ErrAuthExpired(cause error) error {
	return output.ErrWithHint(
		output.ExitAuth, output.TypeAuth,
		fmt.Sprintf("authentication required: %v", cause),
		"run `shoplazza auth login` to refresh credentials",
	)
}

// ErrValidation is a thin theme-package wrapper over output.ErrValidation.
func ErrValidation(format string, args ...any) error {
	return output.ErrValidation(format, args...)
}

// ErrTaskBusinessFailure transports a server-side task=failure into an
// api-class envelope, passing the whole task payload through under the
// "task" extra. Uses the task's "message" field verbatim when present.
func ErrTaskBusinessFailure(task map[string]any) error {
	msg, _ := task["message"].(string)
	if msg == "" {
		msg = "theme task ended with failure"
	}
	return output.Errorf(output.ExitAPI, output.TypeAPI, "%s", msg).
		WithField("task", task)
}

// ErrTaskTimeout is the task-polling cap. Network-class because the cap
// protects against an unresponsive remote, not a server rejection.
// elapsed is the real time spent waiting (rounded to one decimal so test
// assertions are stable); cap is the configured PollOptions.MaxDuration,
// interpolated into the message so non-default callers report the truth.
// The task payload (last observed status) is passed through for triage.
func ErrTaskTimeout(elapsed, cap time.Duration, task map[string]any) error {
	elapsedSec := math.Round(elapsed.Seconds()*10) / 10
	return output.Errorf(output.ExitNetwork, output.TypeNetwork,
		"theme upload task did not finish within %s", cap).
		WithField("elapsed_seconds", elapsedSec).
		WithField("task", task).
		WithHint("task is still running on server; query status manually via the API or wait and retry")
}

// ErrLiveReloadBindFailed flags livereload --port conflicts. Network-class
// because the symptom and remediation mirror a real bind/dial failure.
func ErrLiveReloadBindFailed(port int, cause error) error {
	return output.ErrWithHint(
		output.ExitNetwork, output.TypeNetwork,
		fmt.Sprintf("cannot bind livereload server on port %d: %v", port, cause),
		"another instance may be running; pass --port=<free-port> to override",
	)
}

// ErrWatcherFatal flags an fsnotify crash (EMFILE / loss / perms revoked).
// Internal-class because the remediation is an OS-level config change.
func ErrWatcherFatal(cause error) error {
	return output.ErrWithHint(
		output.ExitInternal, output.TypeInternal,
		fmt.Sprintf("file watcher crashed: %v", cause),
		"increase fs.inotify.max_user_watches (Linux) or check 'ulimit -n' setting; restart `themes serve` after fixing",
	)
}

// ErrLocalIO wraps a local-disk failure (read theme dir, write tmp,
// disk full). Internal-class — the user can't validate around this.
func ErrLocalIO(op string, cause error) error {
	return output.ErrInternal("%s: %v", op, cause)
}

// ErrCloneNetwork is for network-class failures during template clone
// (DNS, dial, TLS handshake, read timeout, connection reset). Distinct
// from ErrLocalIO (disk full, permission, archive extraction), which is
// internal-class.
func ErrCloneNetwork(cause error) error {
	return output.ErrWithHint(
		output.ExitNetwork, output.TypeNetwork,
		fmt.Sprintf("failed to download theme template: %v", cause),
		"check your network connection or proxy settings",
	)
}
