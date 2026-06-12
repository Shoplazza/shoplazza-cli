package doc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"strings"
)

// ErrNotInThemeTree is returned by ParseThemeFile when relPath is not under
// any of the 8 standard theme directories or refers to the directory itself.
var ErrNotInThemeTree = errors.New("file not under any of the standard theme directories")

// themeDirs is intentionally duplicated from internal/theme/pack.ThemeDirs to
// keep this package independent of that one; update both if a 9th dir is added.
var themeDirs = map[string]struct{}{
	"assets": {}, "blocks": {}, "config": {}, "layout": {},
	"locales": {}, "sections": {}, "snippets": {}, "templates": {},
}

// IsEditorTemp reports whether rel (a forward-slash theme-relative path) is an
// editor temp/swap/backup/hidden artifact that must not be synced.
func IsEditorTemp(rel string) bool {
	base := path.Base(rel)
	switch {
	case strings.HasPrefix(base, "."): // hidden, .DS_Store, emacs .#lock
		return true
	case strings.HasSuffix(base, "~"): // emacs/vim backup
		return true
	case strings.HasSuffix(base, ".swp"), strings.HasSuffix(base, ".swo"), strings.HasSuffix(base, ".swx"): // vim swap
		return true
	case strings.HasSuffix(base, ".tmp"):
		return true
	case strings.Contains(base, ".sb-"): // atomic-save temp (e.g. settings_data.json.sb-XXXX)
		return true
	}
	return false
}

// Deduper tracks the last-synced content hash of each file so serve can skip
// re-pushing unchanged content on metadata-only or duplicate fsnotify events.
// NOT safe for concurrent use; callers must serialize externally.
type Deduper struct {
	seen map[string]string // rel (forward-slash) → sha256 hex of last-synced content
}

// NewDeduper returns an empty Deduper.
func NewDeduper() *Deduper { return &Deduper{seen: map[string]string{}} }

// Unchanged reports whether content matches what was last Recorded for rel.
// A rel that was never Recorded is treated as changed (so it syncs).
func (d *Deduper) Unchanged(rel string, content []byte) bool {
	h, ok := d.seen[rel]
	return ok && h == hashContent(content)
}

// Record stores content's hash as rel's last-synced state. Call only AFTER a
// successful push, so a failed sync still retries on the next event.
func (d *Deduper) Record(rel string, content []byte) {
	d.seen[rel] = hashContent(content)
}

// Forget drops rel's recorded hash (call when the file is deleted).
func (d *Deduper) Forget(rel string) { delete(d.seen, rel) }

func hashContent(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ParseThemeFile resolves a theme file relative path into (type, location).
// `type` is one of the 8 standard directories; `location` is the path under
// that directory (always forward-slash, possibly multi-segment). Windows
// backslashes in relPath are normalized to forward slashes.
func ParseThemeFile(relPath string) (string, string, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", "", ErrNotInThemeTree
	}
	// Normalize backslashes explicitly: filepath.ToSlash is a no-op on Unix.
	rel := strings.ReplaceAll(relPath, "\\", "/")
	rel = strings.TrimSuffix(rel, "/")
	if rel == "" {
		return "", "", ErrNotInThemeTree
	}
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		if _, ok := themeDirs[p]; ok {
			if i == len(parts)-1 {
				// pointed at the directory itself
				return "", "", ErrNotInThemeTree
			}
			return p, strings.Join(parts[i+1:], "/"), nil
		}
	}
	return "", "", ErrNotInThemeTree
}

// FileSnapshot is an in-memory file index keyed by theme type ("assets",
// "layout", etc.) with values being the location list within that type.
// NOT safe for concurrent use; callers must serialize access externally.
type FileSnapshot map[string][]string

// FromDocTreeResponse builds a snapshot from a GET /themes/{id}/doctree
// response. Accepts both {"data":{"doctree":{...}}} and {...} shapes.
func FromDocTreeResponse(resp map[string]any) FileSnapshot {
	s := FileSnapshot{}
	doctree := extractDocTree(resp)
	for k, v := range doctree {
		// Normalize the doctree key to the canonical singular type; otherwise
		// pluralized config/layout files are dropped from the snapshot.
		typ := docTreeType(k)
		if _, ok := themeDirs[typ]; !ok {
			continue
		}
		raw, ok := v.([]any)
		if !ok {
			continue
		}
		for _, item := range raw {
			switch it := item.(type) {
			case string:
				// Legacy/alt shape: bare location strings.
				if it != "" {
					s.Add(typ, it)
				}
			case map[string]any:
				// Real doctree shape: {"id": "...", "location": "..."} objects.
				if loc, ok := it["location"].(string); ok && loc != "" {
					s.Add(typ, loc)
				}
			}
		}
	}
	return s
}

// docTreeType maps a doctree response key to the canonical singular theme type,
// normalizing the pluralized configs→config and layouts→layout; other keys are
// returned unchanged.
func docTreeType(key string) string {
	switch key {
	case "configs":
		return "config"
	case "layouts":
		return "layout"
	default:
		return key
	}
}

func extractDocTree(resp map[string]any) map[string]any {
	if data, ok := resp["data"].(map[string]any); ok {
		if dt, ok := data["doctree"].(map[string]any); ok {
			return dt
		}
		// Some envelopes put theme dirs directly under data.
		return data
	}
	return resp
}

// Has reports whether (typ, location) is tracked in the snapshot.
func (s FileSnapshot) Has(typ, location string) bool {
	for _, l := range s[typ] {
		if l == location {
			return true
		}
	}
	return false
}

// Add inserts (typ, location); dedupes silently.
func (s FileSnapshot) Add(typ, location string) {
	if s.Has(typ, location) {
		return
	}
	s[typ] = append(s[typ], location)
}

// Remove deletes (typ, location) if present.
func (s FileSnapshot) Remove(typ, location string) {
	list := s[typ]
	for i, l := range list {
		if l == location {
			s[typ] = append(list[:i], list[i+1:]...)
			return
		}
	}
}
