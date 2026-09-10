// Package theme provides error-classification helpers shared by the
// theme subpackages and the cmd/themes/* shortcut layer. Every helper
// maps to exactly one of the five envelope types
// (api/validation/auth/network/internal).
package theme

import (
	"fmt"
	"math"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
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

// ErrTaskTimeout is the task-polling cap error; it carries the task id and
// last payload so the wait can be resumed with --task-id.
func ErrTaskTimeout(elapsed, cap time.Duration, taskID string, task map[string]any) error {
	elapsedSec := math.Round(elapsed.Seconds()*10) / 10
	return output.Errorf(output.ExitNetwork, output.TypeNetwork,
		"theme upload task did not finish within %s", cap).
		WithField("elapsed_seconds", elapsedSec).
		WithField("task_id", taskID).
		WithField("task", task).
		WithHint(fmt.Sprintf("task is still running on server; re-run with --task-id %s to keep waiting, or check it with 'shoplazza themes task'", taskID))
}

// ErrTaskInterrupted reports a locally canceled wait; the task keeps running on the server.
func ErrTaskInterrupted(taskID string) error {
	return output.ErrWithHint(output.ExitNetwork, output.TypeNetwork,
		fmt.Sprintf("stopped waiting for theme upload task %s", taskID),
		fmt.Sprintf("the task keeps running on the server; re-run with --task-id %s to keep waiting", taskID))
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
