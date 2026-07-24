package themes

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// Target engine for themes +page / +edit. The target string is the single
// block coordinate the contract exposes (docs/theme-page-edit-shortcuts.md
// §3.4): +page pre-builds one per flattened block row, +edit parses them back
// into the parent_path/block_index coordinates the write endpoints take.
// Both directions live here so the path syntax has exactly one owner.

// FlatBlock is one row of the depth-first flattened blocks list returned by
// themes +page.
type FlatBlock struct {
	Type     string
	Settings map[string]any
	Target   string
}

// flattenBlocks flattens a section's blocks tree depth-first: parent row
// first, its children immediately after (render order). Only the `blocks` key
// recurses — arrays inside settings are field values and stay untouched.
func flattenBlocks(sectionID string, blocks []any) []FlatBlock {
	var out []FlatBlock
	var walk func(prefix string, items []any)
	walk = func(prefix string, items []any) {
		for i, it := range items {
			m := asMap(it)
			if m == nil {
				continue
			}
			target := fmt.Sprintf("%s.blocks[%d]", prefix, i)
			settings, _ := m["settings"].(map[string]any)
			out = append(out, FlatBlock{Type: getString(m, "type"), Settings: settings, Target: target})
			if kids, ok := m["blocks"].([]any); ok && len(kids) > 0 {
				walk(target, kids)
			}
		}
	}
	walk(sectionID, blocks)
	return out
}

type targetKind int

const (
	targetSection   targetKind = iota // "<section_id>"
	targetBlock                       // "<section_id>.blocks[1]", nested allowed
	targetContainer                   // "<section_id>.blocks", "<section_id>.blocks[0].blocks"
)

// targetRef is a parsed target. For targetBlock, ParentPath holds the ancestor
// indexes from the section root down to the target's parent and BlockIndex the
// position within it — exactly the write endpoints' body coordinates. For
// targetContainer, ParentPath addresses the container itself (append target).
type targetRef struct {
	SectionID  string
	ParentPath []int
	BlockIndex int // -1 unless Kind == targetBlock
	Kind       targetKind
}

// parseTarget parses a target string. Grammar:
//
//	target    = section_id suffix
//	suffix    = (".blocks[" index "]")* [".blocks"]
func parseTarget(target string) (targetRef, error) {
	ref := targetRef{BlockIndex: -1, Kind: targetSection}
	idx := strings.Index(target, ".blocks")
	if idx == -1 {
		if target == "" {
			return ref, invalidTarget(target, "empty section id")
		}
		ref.SectionID = target
		return ref, nil
	}
	ref.SectionID = target[:idx]
	if ref.SectionID == "" {
		return ref, invalidTarget(target, "empty section id")
	}
	rest := target[idx:]
	var idxs []int
	container := false
	for len(rest) > 0 {
		if container {
			return ref, invalidTarget(target, "no segments allowed after a bare .blocks container suffix")
		}
		if !strings.HasPrefix(rest, ".blocks") {
			return ref, invalidTarget(target, "expected a .blocks segment")
		}
		rest = rest[len(".blocks"):]
		if strings.HasPrefix(rest, "[") {
			end := strings.Index(rest, "]")
			if end <= 1 {
				return ref, invalidTarget(target, "malformed block index")
			}
			n, err := strconv.Atoi(rest[1:end])
			if err != nil || n < 0 {
				return ref, invalidTarget(target, "malformed block index")
			}
			idxs = append(idxs, n)
			rest = rest[end+1:]
		} else {
			container = true
		}
	}
	if container {
		ref.Kind = targetContainer
		ref.ParentPath = idxs
		return ref, nil
	}
	ref.Kind = targetBlock
	ref.ParentPath = idxs[:len(idxs)-1]
	ref.BlockIndex = idxs[len(idxs)-1]
	return ref, nil
}

func invalidTarget(target, reason string) error {
	return output.ErrValidation("invalid target %q: %s", target, reason).
		WithHint("copy targets verbatim from the `themes +page` output; append .blocks to a parent target to address its container")
}
