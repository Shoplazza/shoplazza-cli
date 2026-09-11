package registry

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

// resetLoadSpec discards the cached spec so tests that swap Embedded can
// verify parse-failure and initialisation behaviour.
func resetLoadSpec() {
	loadOnce = sync.Once{}
	activeSpec = nil
	specSource = ""
	embeddedRevOnce = sync.Once{}
	embeddedRev = ""
}

func TestLoadSpec_EmbeddedSpec(t *testing.T) {
	s := LoadSpec()
	if s == nil {
		t.Fatal("LoadSpec must never return nil")
	}
	if s.Version == "" {
		t.Fatal("expected non-empty version on embedded spec")
	}
	if len(s.Modules) == 0 {
		t.Fatal("expected embedded spec to declare at least one module")
	}
	if s.Schemas == nil {
		t.Fatal("Schemas must be a non-nil map after Load")
	}
}

// TestLoadSpec_DegradesOnCorruptInput verifies LoadSpec yields an empty Spec
// rather than failing when the embedded JSON is corrupt.
func TestLoadSpec_DegradesOnCorruptInput(t *testing.T) {
	saved := Embedded
	t.Cleanup(func() {
		Embedded = saved
		resetLoadSpec() // clear cache so subsequent tests reload the real spec
	})

	resetLoadSpec() // discard any spec cached by earlier tests
	Embedded = []byte("{not valid json")
	s := LoadSpec()
	if s == nil {
		t.Fatal("LoadSpec must never return nil even on corrupt input")
	}
	if len(s.Modules) != 0 {
		t.Fatal("corrupt input must yield empty modules")
	}
}

func TestLoadSpec_NormalizesNilSchemasMap(t *testing.T) {
	saved := Embedded
	t.Cleanup(func() {
		Embedded = saved
		resetLoadSpec()
	})

	resetLoadSpec()
	Embedded = []byte(`{"version":"v","modules":[]}`)
	s := LoadSpec()
	if s.Schemas == nil {
		t.Fatal("Schemas must be non-nil even when JSON omits it")
	}
}

func generateLargeSpec(b *testing.B) []byte {
	b.Helper()
	s := Spec{Version: "bench"}
	for i := 0; i < 100; i++ {
		m := Module{Name: fmt.Sprintf("svc-%d", i)}
		for j := 0; j < 5; j++ {
			m.Commands = append(m.Commands, Command{
				ID:   fmt.Sprintf("svc%d-method%d", i, j),
				Path: []string{fmt.Sprintf("method-%d", j)},
				HTTP: HTTP{Method: "GET", Path: fmt.Sprintf("/svc%d/method%d", i, j)},
			})
		}
		s.Modules = append(s.Modules, m)
	}
	out, err := json.Marshal(s)
	if err != nil {
		b.Fatal(err)
	}
	return out
}

func BenchmarkLoadSpec_LargeFixture(b *testing.B) {
	fixture := generateLargeSpec(b)
	saved := Embedded
	b.Cleanup(func() { Embedded = saved })
	Embedded = fixture
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LoadSpec()
	}
}
