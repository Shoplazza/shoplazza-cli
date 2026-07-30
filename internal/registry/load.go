package registry

import (
	"encoding/json"
	"sync"
)

var (
	loadOnce   sync.Once
	activeSpec *Spec
	specSource string
)

// LoadSpec returns the active spec, preferring a valid newer downloaded
// cache over the embedded copy, caching the result. It never returns nil;
// corrupt input yields an empty Spec so the CLI can still start.
func LoadSpec() *Spec {
	loadOnce.Do(func() {
		spec, source := &Spec{}, SourceEmbedded
		if cached := loadCachedSpec(); cached != nil {
			spec, source = cached, SourceCached
		} else if err := json.Unmarshal(Embedded, spec); err != nil {
			spec = &Spec{}
		}
		if spec.Schemas == nil {
			spec.Schemas = map[string]ObjectSchema{}
		}
		// Build module name → slice index for O(1) findModule lookups.
		spec.moduleIndex = make(map[string]int, len(spec.Modules))
		for i, m := range spec.Modules {
			spec.moduleIndex[m.Name] = i
		}
		activeSpec, specSource = spec, source
	})
	return activeSpec
}

// SpecSource reports where the active spec came from: SourceEmbedded or
// SourceCached.
func SpecSource() string {
	LoadSpec()
	return specSource
}
