package common

import "fmt"

// ValidateShortcut returns an error if s is missing required declarative fields.
// Called by shortcuts/declaration_test.go to fail the build on incomplete
// Shortcut literals.
//
// Exactly one of Plan or Execute must be set: Plan for single-step shortcuts,
// Execute for multi-step orchestration shortcuts.
func ValidateShortcut(s Shortcut) error {
	if s.Service == "" {
		return fmt.Errorf("shortcut %q: Service is empty", s.Command)
	}
	if s.Command == "" {
		return fmt.Errorf("shortcut under service %q: Command is empty", s.Service)
	}
	hasPlan := s.Plan != nil
	hasExec := s.Execute != nil
	switch {
	case !hasPlan && !hasExec:
		return fmt.Errorf("shortcut %q %q: Plan and Execute are both nil; set one", s.Service, s.Command)
	case hasPlan && hasExec:
		return fmt.Errorf("shortcut %q %q: Plan and Execute are both set; set exactly one", s.Service, s.Command)
	}
	return nil
}
