package output

import (
	"fmt"
	"io"
)

// WriteError renders err to w for the given resolved output format.
//
// A human watching a terminal (a pretty/table format AND w is a real TTY) gets a
// readable, optionally-colored line. Every machine case gets the JSON error
// envelope: an explicit json/ndjson/csv format, OR a non-terminal stderr (piped,
// redirected, CI). Both conditions must hold for the human path, so the "agents
// parse stderr as JSON" contract survives even when stdout is a pretty pty but
// stderr is piped — the writer decides, not just the format.
func WriteError(w io.Writer, err *ExitError, format string) {
	if err == nil {
		return
	}
	if isHumanFormat(format) && IsTerminal(w) {
		writeErrorHuman(w, err, colorEnabled(w))
		return
	}
	WriteErrorEnvelope(w, err)
}

// isHumanFormat reports whether format is one of the human-oriented renderers.
// ndjson/csv are export/stream formats and stay on the JSON envelope path.
func isHumanFormat(format string) bool {
	return format == FormatPretty || format == FormatTable
}

// writeErrorHuman prints a gh/docker-style error: an "Error:" line, then the
// hint and any request context, one per line. Message/hint fall back to the
// error string when no structured Detail is present.
func writeErrorHuman(w io.Writer, e *ExitError, color bool) {
	msg := e.Error()
	hint, code := "", ""
	var ctx *ErrorContext
	if e.Detail != nil {
		if e.Detail.Message != "" {
			msg = e.Detail.Message
		}
		hint = e.Detail.Hint
		code = e.Detail.Code
		ctx = e.Detail.Detail
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", colorErrLabel("Error:", color), msg)
	// The API error code and HTTP status carry the machine-branchable identity a
	// human needs to look up or report the error; surface them when present (the
	// JSON envelope already has them, pretty used to drop them).
	if line := codeStatusLine(code, ctx, color); line != "" {
		_, _ = io.WriteString(w, line)
	}
	if hint != "" {
		_, _ = fmt.Fprintf(w, "%s %s\n", colorDim("Hint:", color), hint)
	}
	if ctx != nil {
		if ctx.Method != "" && ctx.Path != "" {
			_, _ = fmt.Fprintf(w, "%s %s %s\n", colorDim("Request:", color), ctx.Method, ctx.Path)
		}
		if ctx.RequestID != "" {
			_, _ = fmt.Fprintf(w, "%s %s\n", colorDim("Request ID:", color), ctx.RequestID)
		}
	}
}

// codeStatusLine renders the API error code and/or HTTP status when either is
// present: "Code: X (HTTP 400)", "Code: X", or "HTTP status: 400". Returns "" if
// neither is set (local/validation errors that never hit the network).
func codeStatusLine(code string, ctx *ErrorContext, color bool) string {
	status := 0
	if ctx != nil {
		status = ctx.StatusCode
	}
	switch {
	case code != "" && status != 0:
		return fmt.Sprintf("%s %s (HTTP %d)\n", colorDim("Code:", color), code, status)
	case code != "":
		return fmt.Sprintf("%s %s\n", colorDim("Code:", color), code)
	case status != 0:
		return fmt.Sprintf("%s %d\n", colorDim("HTTP status:", color), status)
	default:
		return ""
	}
}

// colorErrLabel renders the "Error:" label bold-red when color is enabled.
func colorErrLabel(s string, color bool) string {
	if !color {
		return s
	}
	return ansiBold + ansiRed + s + ansiReset
}
