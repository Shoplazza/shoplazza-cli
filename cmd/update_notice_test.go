package cmd

import "testing"

func TestIsUpdateCheckSkippedCommand(t *testing.T) {
	skipped := [][]string{
		{"update"},
		{"completion", "bash"},
		{"__complete", "products", ""},
		{"--format", "json", "update"},
	}
	for _, args := range skipped {
		if !isUpdateCheckSkippedCommand(args) {
			t.Errorf("args %v should be skipped", args)
		}
	}
	notSkipped := [][]string{
		{"products", "list"},
		{"auth", "login"},
		{},
		{"app", "deploy"},
	}
	for _, args := range notSkipped {
		if isUpdateCheckSkippedCommand(args) {
			t.Errorf("args %v should not be skipped", args)
		}
	}
}
