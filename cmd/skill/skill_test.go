package skill

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/skillcontent"
)

// withFakeSkills registers an in-memory skill FS for the duration of the test.
func withFakeSkills(t *testing.T) {
	t.Helper()
	prev := skillcontent.Embedded()
	skillcontent.SetFS(fstest.MapFS{
		"shoplazza-orders/SKILL.md":              {Data: []byte("---\nname: shoplazza-orders\ndescription: Manage orders.\n---\n\n# Orders\n")},
		"shoplazza-orders/references/refunds.md": {Data: []byte("# Refunds guide\n")},
	})
	t.Cleanup(func() { skillcontent.SetFS(prev) })
}

// run executes the skills command tree with args, returning stdout and error.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewCmdSkill()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestListEmitsEmbeddedSkills(t *testing.T) {
	withFakeSkills(t)
	out, err := run(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "shoplazza-orders") || !strings.Contains(out, "\"count\": 1") {
		t.Errorf("list output missing skill/count: %s", out)
	}
}

func TestReadPrintsRawMarkdown(t *testing.T) {
	withFakeSkills(t)
	out, err := run(t, "read", "shoplazza-orders")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasPrefix(out, "---\nname: shoplazza-orders") || !strings.Contains(out, "# Orders") {
		t.Errorf("read should print raw SKILL.md, got: %s", out)
	}
}

func TestReadReferenceViaSecondArg(t *testing.T) {
	withFakeSkills(t)
	out, err := run(t, "read", "shoplazza-orders", "references/refunds.md")
	if err != nil {
		t.Fatalf("read ref: %v", err)
	}
	if !strings.Contains(out, "# Refunds guide") {
		t.Errorf("want refunds reference, got: %s", out)
	}
}

func TestReadReferenceViaSlashPath(t *testing.T) {
	withFakeSkills(t)
	out, err := run(t, "read", "shoplazza-orders/references/refunds.md")
	if err != nil {
		t.Fatalf("read slash-path: %v", err)
	}
	if !strings.Contains(out, "# Refunds guide") {
		t.Errorf("want refunds reference, got: %s", out)
	}
}

func TestReadRejectsRefGivenTwice(t *testing.T) {
	withFakeSkills(t)
	if _, err := run(t, "read", "shoplazza-orders/references", "refunds.md"); err == nil {
		t.Error("passing the ref path both via slash and second arg should error")
	}
}

func TestReadUnknownSkillErrors(t *testing.T) {
	withFakeSkills(t)
	if _, err := run(t, "read", "does-not-exist"); err == nil {
		t.Error("reading an unknown skill should error")
	}
}
