package common

import (
	"context"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
)

// ExecInput is what the engine hands to a Shortcut's Execute function for
// multi-step orchestration commands. Unlike PlanInput, it carries a client
// so the function can perform intermediate GETs before deciding the final
// request shape.
type ExecInput struct {
	Args   []string
	Flags  FlagSet
	Tool   string
	Client *client.Client
	DryRun bool
}

// ExecResult is what an Execute function returns to the engine. In dry-run mode
// populate Plans with every intended HTTP step and leave Body nil; in live mode
// populate Body with the final response payload to wrap in the success envelope.
type ExecResult struct {
	Plans []PlannedRequest
	Body  map[string]any
}

// ExecuteFunc is the signature of Shortcut.Execute. Exposed as a named type
// so tests and stubs can reference it.
type ExecuteFunc func(ctx context.Context, in ExecInput) (ExecResult, error)
