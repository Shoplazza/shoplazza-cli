package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// LocalExt is a local extension discovered under extensions/<Dir>.
type LocalExt struct {
	Dir         string
	Name        string
	Type        string
	Version     string // optional; from the extension toml. Diff ignores it.
	ExtensionID string // optional; set once known
	AppID       string // v1 extension.config.json `appId`; empty for v2. Diff ignores it.
}

// Pair maps a local extension to its remote counterpart. Remote is nil when the
// local extension is new (to be created).
type Pair struct {
	Local  LocalExt
	Remote *Extension
}

// Diff matches local extensions to remote extension_versions deterministically
// (no interactive disambiguation). Rules applied in order:
//  1. exact extension_id
//  2. exact (name, type)
//  3. single unmatched local of a type + single unmatched remote of that type → match
//  4. any type with unmatched local(s) AND remaining unmatched remote(s) → validation error
//
// A local whose type has zero unmatched remotes is NEW (Remote == nil).
func Diff(locals []LocalExt, remotes []Extension) ([]Pair, *output.ExitError) {
	pairs := make([]Pair, len(locals))
	matched := make([]bool, len(locals))
	usedRemote := make([]bool, len(remotes))
	for i := range locals {
		pairs[i] = Pair{Local: locals[i]}
	}

	// Pass 1: exact extension_id.
	for i, l := range locals {
		if l.ExtensionID == "" {
			continue
		}
		for j := range remotes {
			if !usedRemote[j] && remotes[j].ExtensionID == l.ExtensionID {
				pairs[i].Remote = &remotes[j]
				matched[i], usedRemote[j] = true, true
				break
			}
		}
	}

	// Pass 2: exact (name, type).
	for i, l := range locals {
		if matched[i] {
			continue
		}
		for j := range remotes {
			if !usedRemote[j] && remotes[j].ExtensionType == l.Type && remotes[j].ExtensionName == l.Name {
				pairs[i].Remote = &remotes[j]
				matched[i], usedRemote[j] = true, true
				break
			}
		}
	}

	// Pass 3: single unmatched local of a type + single unmatched remote of that type.
	localsByType := map[string][]int{}
	for i := range locals {
		if !matched[i] {
			localsByType[locals[i].Type] = append(localsByType[locals[i].Type], i)
		}
	}
	remotesByType := map[string][]int{}
	for j := range remotes {
		if !usedRemote[j] {
			remotesByType[remotes[j].ExtensionType] = append(remotesByType[remotes[j].ExtensionType], j)
		}
	}
	for typ, li := range localsByType {
		rj := remotesByType[typ]
		if len(li) == 1 && len(rj) == 1 {
			pairs[li[0]].Remote = &remotes[rj[0]]
			matched[li[0]], usedRemote[rj[0]] = true, true
		}
	}

	// Pass 4: any type with unmatched local(s) AND remaining unmatched remote(s)
	// is ambiguous. A type with 0 remaining remotes => locals are NEW (Remote nil).
	remainLocals := map[string][]string{}
	for i := range locals {
		if !matched[i] {
			remainLocals[locals[i].Type] = append(remainLocals[locals[i].Type], locals[i].Name)
		}
	}
	remainRemotes := map[string]int{}
	for j := range remotes {
		if !usedRemote[j] {
			remainRemotes[remotes[j].ExtensionType]++
		}
	}
	var ambig []string
	for typ, names := range remainLocals {
		if remainRemotes[typ] > 0 {
			sort.Strings(names)
			ambig = append(ambig, fmt.Sprintf("%s: local[%s] vs %d unmatched remote(s)", typ, strings.Join(names, ","), remainRemotes[typ]))
		}
	}
	if len(ambig) > 0 {
		sort.Strings(ambig)
		return nil, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"ambiguous extension match — "+strings.Join(ambig, "; "),
			"rename local extensions to match the remote (name,type), or set extension_id to map explicitly")
	}
	return pairs, nil
}
