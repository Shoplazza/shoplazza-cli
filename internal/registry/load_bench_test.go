package registry

import (
	"encoding/json"
	"fmt"
	"testing"
)

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
