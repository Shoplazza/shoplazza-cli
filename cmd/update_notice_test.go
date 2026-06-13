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
			t.Errorf("args %v 应被跳过", args)
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
			t.Errorf("args %v 不应被跳过", args)
		}
	}
}
