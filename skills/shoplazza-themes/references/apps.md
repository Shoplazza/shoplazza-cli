# themes apps — app embeds and script tags: read their state, turn them on or off

A theme carries two kinds of app switches:

- **App embeds** — app blocks embedded in the theme (popup, announcement bar, floating button,
  product customizer…).
- **Script tags** — scripts an app injects into the storefront (translation, tracking, custom
  scripts…).

"Install app X" is three separate things, and only the second is here:

1. Install, authorize, subscribe or uninstall in the app store → merchant admin, not a CLI action.
   Say so.
2. Turn its switch on or off in the theme → this file.
3. Put its extension card on a page → `themes app extensions` lists those cards; add one with
   [card-add.md](card-add.md).

A vague "set up app X" → explain this split first, then do step 2.

## Commands

```bash
# Catalog: what the installed apps offer (names, kinds). Shows no on/off state.
themes app list --params '{"limit":50}'

# The template file id of the homepage (doc_id for section list)
themes file tree --params '{"theme_id":"<theme_id>"}' --jq '.data.templates[] | select(.location=="index.liquid") | .id'

# Current state in this session
themes section list --params '{"oseid":"<oseid>","doc_id":"<index_doc_id>"}' --jq '.data.data.sections | {app_embeds, script_tags}'

# Turn on / off (draft write into the session)
themes app enable  --params '{"oseid":"<oseid>"}' --data '{"theme_id":"<theme_id>","block_id":"<block_id>"}'
themes app disable --params '{"oseid":"<oseid>"}' --data '{"theme_id":"<theme_id>","block_id":"<block_id>"}'

# Extension cards that can be added to a page
themes app extensions --params '{"template":"<template>"}'
```

`section list` needs the template file's uuid from `file tree`; `"index"` does not work there.
`themes app extensions` → `.data.apps[]` (`id` = app key) with `blocks[{name, type, settings, source}]`;
a block's `type` is what `add_section` takes as `name`.

## Where the state lives

`app list` returns `app_embeds[]` (`name` multilingual, `type`, `target`, `settings`…) and
`script_tag_apps[]` (`app_info{name, uid,…}`, `script_tags[{id, app_id, name,…}]`). It takes no
session and does not show whether an item is on in this theme — never read on/off from it.

The on/off state is per session, in `section list` → `.data.data.sections`:

| | App embed | Script tag |
|---|---|---|
| Location | `app_embeds.<app_key>[]` (`app_key` like `publicapp`) | `script_tags.<app_uid>[]` |
| Entry | `{type, disabled, settings}` | `{id, disabled, settings}` |
| On / off | `disabled:false` / `disabled:true` | `disabled:false` / `disabled:true` |
| `block_id` to send | the stored `type` (`shoplazza://apps/…/blocks/<handle>/<id>`) | the stored `id` (uuid) |
| `category` (optional) | `app_embeds` | `script_tags` |

To tell which stored entry is the app the user means, compare with `app list`: an embed's
`<handle>` (the segment after `/blocks/`, e.g. `popup`) and its `name`; a script tag's app via
`app_info.uid` / `app_id`. The `type` in `app list` can differ from the one stored in the session
(different trailing id) — send the stored one.

## Flow

1. Theme and session: SKILL.md → Rules for every operation (1, 2).
2. `app list` → match the user's wording to an app by name. No match → list the candidates and
   ask; never guess a `block_id`.
3. `section list` → find its stored entry and current state.
   - Already in the requested state → no call; say so.
   - Not stored in this session → it can't be switched from here (the server rejects ids it hasn't
     stored, with a 500); say so — the theme editor's app embeds panel is the alternative. Don't try
     ids from `app list` instead.
   - An `app list` entry with `allowChangeDisabled:false` can't be changed — say so.
4. `themes app enable` / `themes app disable` with the stored id, verbatim. `category` and `app_key` only to
   disambiguate a `block_id` that occurs twice; `doc_id` is optional (the server uses the
   homepage file when omitted).
5. **Re-read `section list`** and report what it shows. A `{}` success does not prove the switch
   flipped: an app-embed enable has returned `{}` while the entry stayed `disabled:true`. State
   unchanged → report that the switch did not take effect.
6. Give the preview (`themes +preview -t <theme_id> --oseid <oseid>`) and stop.

Announcement-bar requests are often an app embed — check `app list` first. A theme's own
announcement card is read with `+page --area global` ([page-read.md](page-read.md)) and edited
like any card ([card-edit.md](card-edit.md)).

## Rules

- Take `block_id` verbatim from the stored entry: an embed's `type` and a script tag's `id` are
  different fields, and mixing them switches the wrong object.
- Draft write: runs directly, no consent. Saving and publishing are separate
  (SKILL.md → Workflow — editing a theme page, steps 7–8).

## Output

- Listing: plain text, one line per item — name, kind (app embed / script tag), state (on / off /
  not in this theme / not changeable). Both lists empty → the theme has no app switches.
- After a write: "turned on / off <name>" only if the re-read confirms it, plus the preview link.
- `app enable|disable` have no `--promote`: they share the session draft with card edits; save or
  publish with `themes +edit -t <theme_id> --template index --session <oseid> --ops '[]' --promote [--publish]`.

## Errors & recovery

| Error | Meaning / fix |
|---|---|
| 500 `internal server error` on enable/disable | The server looks the `block_id` up in the entries stored for this theme before switching anything; an id that isn't stored there (ids from `app list` often differ in the trailing number, and embeds never switched on in this theme aren't stored at all) fails with this 500 and nothing changes. Use the stored id; if the app isn't stored, it can't be switched from the CLI — say so and point to the theme editor. Don't retry with guessed ids |
| `{}` but state unchanged on re-read | The switch didn't take effect — report it; suggest the theme editor |
| 404 `b_record_not_found` on `section list` | `doc_id` must be the template file uuid from `file tree`, not `"index"` |
