package main

import (
	"embed"
	"io/fs"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/skillcontent"
)

// skillsFS embeds only the domain skill content (SKILL.md + references) into the
// binary, so `shoplazza skills read/list` serves guidance that matches this
// exact CLI version. The eval harness (shoplazza-skill-eval) and _template are
// deliberately excluded — they are tooling, not agent-facing content. New domain
// skills must be added to this list.
//
//go:embed skills/shoplazza-billing skills/shoplazza-common skills/shoplazza-customers skills/shoplazza-discounts skills/shoplazza-orders skills/shoplazza-products skills/shoplazza-shop skills/shoplazza-webhook
var skillsFS embed.FS

func init() {
	// Strip the leading "skills/" so the reader's top-level entries are the
	// skill names (e.g. "shoplazza-orders/SKILL.md").
	if sub, err := fs.Sub(skillsFS, "skills"); err == nil {
		skillcontent.SetFS(sub)
	}
}
