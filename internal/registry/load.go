package registry

import (
	"encoding/json"
	"sync"
)

var (
	loadOnce   sync.Once
	cachedSpec *Spec
)

// LoadSpec parses the embedded cli_meta.json, caching the result. It never
// returns nil; corrupt JSON yields an empty Spec so the CLI can still start.
func LoadSpec() *Spec {
	loadOnce.Do(func() {
		var spec Spec
		if err := json.Unmarshal(Embedded, &spec); err != nil {
			cachedSpec = &Spec{}
			return
		}
		if spec.Schemas == nil {
			spec.Schemas = map[string]ObjectSchema{}
		}
		// Build module name → slice index for O(1) findModule lookups.
		spec.moduleIndex = make(map[string]int, len(spec.Modules))
		for i, m := range spec.Modules {
			spec.moduleIndex[m.Name] = i
		}
		cachedSpec = &spec
	})
	return cachedSpec
}
