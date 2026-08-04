package cmd

import "testing"

func TestIsUpdateCheckSkippedCommand(t *testing.T) {
	root := NewRootCmd()
	skipped := [][]string{
		{"update"},
		{"update", "--check"},
		{"completion", "bash"},
		{"__complete", "products", ""},
		{"--format", "json", "update"},
	}
	for _, args := range skipped {
		if !isUpdateCheckSkippedCommand(root, args) {
			t.Errorf("args %v should be skipped", args)
		}
	}
	notSkipped := [][]string{
		{"products", "list"},
		{"products", "update", "--params", "{}"}, // dynamic leaf named update
		{"auth", "login"},
		{},
		{"app", "deploy"},
		{"no-such-command"},
	}
	for _, args := range notSkipped {
		if isUpdateCheckSkippedCommand(root, args) {
			t.Errorf("args %v should not be skipped", args)
		}
	}
}
