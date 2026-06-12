package registry

// Spec is the root structure embedded as cli_meta.json.
type Spec struct {
	Version     string                  `json:"version"`
	GeneratedAt string                  `json:"generated_at"`
	Modules     []Module                `json:"modules"`
	Schemas     map[string]ObjectSchema `json:"schemas,omitempty"`
	moduleIndex map[string]int          // built by LoadSpec; O(1) module lookup; not serialised
}

// Module groups commands under a single top-level cobra name (kebab-case).
// Groups carries per-implicit-subgroup metadata keyed by the kebab-case name
// (e.g. "variants" → metadata for `products variants`); the top-level
// module's Short/Long is supplied by the dynamic builder, not the spec.
type Module struct {
	Name     string                   `json:"name"`
	Groups   map[string]GroupMetadata `json:"groups,omitempty"`
	Commands []Command                `json:"commands"`
}

// GroupMetadata is the Short/Long source for an implicit subgroup synthesised
// from commands[].path (e.g. `products variants`). Both fields are optional.
type GroupMetadata struct {
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
}

// Command is one HTTP endpoint exposed as a cobra leaf.
type Command struct {
	ID             string      `json:"id"`
	Path           []string    `json:"command_path"`
	Summary        string      `json:"summary"`
	Description    string      `json:"description,omitempty"`
	Hidden         bool        `json:"hidden,omitempty"`
	HTTP           HTTP        `json:"http"`
	Parameters     []Parameter `json:"parameters,omitempty"`
	Body           *Body       `json:"body,omitempty"`
	ResponseSchema string      `json:"response_schema,omitempty"`
}

type HTTP struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"` // M2 only "*"; "" otherwise
}

type Body struct {
	Required bool    `json:"required,omitempty"`
	Fields   []Field `json:"fields"`
}

type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"` // "path" or "query"
	Type        string `json:"type"`
	Format      string `json:"format,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Minimum     any    `json:"minimum,omitempty"`
	Maximum     any    `json:"maximum,omitempty"`
	MinLength   *int   `json:"min_length,omitempty"`
	MaxLength   *int   `json:"max_length,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
	Items       *Field `json:"items,omitempty"`
	Schema      string `json:"schema,omitempty"`
}

type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Format      string `json:"format,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Minimum     any    `json:"minimum,omitempty"`
	Maximum     any    `json:"maximum,omitempty"`
	MinLength   *int   `json:"min_length,omitempty"`
	MaxLength   *int   `json:"max_length,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
	Items       *Field `json:"items,omitempty"`
	Schema      string `json:"schema,omitempty"`
}

type ObjectSchema struct {
	Fields []Field `json:"fields"`
}
