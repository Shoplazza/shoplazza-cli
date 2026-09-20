package themecmd

import "testing"

// TestValidateInitName pins that --name must be a plain directory segment: path
// separators, "..", "." and absolute paths are rejected before any clone (a
// traversal name previously extracted outside the cwd).
func TestValidateInitName(t *testing.T) {
	for _, bad := range []string{"../x", "a/b", `a\b`, "/abs", "..", "."} {
		if err := validateInitName(bad); err == nil {
			t.Errorf("name %q must be rejected", bad)
		}
	}
	for _, ok := range []string{"my-theme", "Nova2023", "theme_1"} {
		if err := validateInitName(ok); err != nil {
			t.Errorf("name %q must be accepted: %v", ok, err)
		}
	}
}
