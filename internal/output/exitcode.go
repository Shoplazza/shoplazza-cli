package output

// Exit codes used by the CLI. Agents parse these to classify errors without
// inspecting the error message text.
const (
	ExitOK         = 0 // success
	ExitAPI        = 1 // API / generic error
	ExitValidation = 2 // invalid flag or argument
	ExitAuth       = 3 // unauthenticated or token expired
	ExitNetwork    = 4 // network unreachable or timeout
	ExitInternal   = 5 // unexpected internal error
)

// Error type strings serialized as ErrDetail.Type. Each pairs naturally with
// one of the exit codes above (api↔ExitAPI, validation↔ExitValidation, etc.).
// Use these constants everywhere — bare string literals risk silent typos.
const (
	TypeAPI        = "api"
	TypeValidation = "validation"
	TypeAuth       = "auth"
	TypeNetwork    = "network"
	TypeInternal   = "internal"
)
