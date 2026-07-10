package core

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateConfig_ReadMutateWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := SaveConfig(p, CliConfig{ConfigVersion: 2, CurrentProfile: "a"}); err != nil {
		t.Fatal(err)
	}
	err := UpdateConfig(p, time.Second, func(c *CliConfig) error {
		c.CurrentProfile = "b"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := LoadConfig(p)
	if got.CurrentProfile != "b" {
		t.Fatalf("mutate lost: %+v", got)
	}
}

func TestUpdateConfig_MutateErrAbortsWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	_ = SaveConfig(p, CliConfig{ConfigVersion: 2, CurrentProfile: "a"})
	wantErr := errors.New("boom")
	if err := UpdateConfig(p, time.Second, func(*CliConfig) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("err passthrough: %v", err)
	}
	got, _ := LoadConfig(p)
	if got.CurrentProfile != "a" {
		t.Fatal("must not write on mutate error")
	}
}
