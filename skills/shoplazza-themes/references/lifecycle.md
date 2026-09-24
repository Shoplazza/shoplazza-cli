# themes lifecycle — rename, duplicate, upgrade, delete, publish

Whole-theme writes on an installed theme. Each is one call that takes effect at once — no edit
session, no save step. `delete` and `publish` need consent first (SKILL.md → Rules for every
operation, rule 8); rename, duplicate and upgrade run directly.

## Commands

| Intent | Command | Returns |
|---|---|---|
| Rename | `themes rename --params '{"theme_id":"<id>"}' --data '{"name":"<new name>"}'` | `.data.data.{msg, state}` only — no theme object |
| Duplicate | `themes duplicate --data '{"copy_theme_id":"<source id>","copy_theme_name":"<new name>"}'` | A new theme (see Errors: can fail with 500 yet create it) |
| Upgrade | `themes upgrade --params '{"theme_id":"<id>"}' [--data '{"name":"<new name>"}']` | The new theme's id (`schema themes.upgrade` → `theme_id`) |
| Delete (consent) | `themes delete --params '{"theme_id":"<id>"}'` | `{}` |
| Publish (consent) | `themes publish --params '{"theme_id":"<id>"}'` | `.data.theme.{published, publish_time, revoke_publish_id?}` |

`duplicate` is the only one whose source id goes in `--data` (`copy_theme_id`), not `--params`.
Omit `sources` on duplicate — it copies everything by default.

## Flow

1. **Locate the theme** — SKILL.md → Rules for every operation (rule 1); by name →
   [query.md](query.md) → Finding a theme by name. Only the "which theme" half of the request
   locates it. In "rename Reformia to Reformia Black Friday" the new name is data to write, never
   a search term: don't match it against the list, and never answer "no such theme" because the
   new name isn't found.
2. **Pre-check** against the table below using a fresh `themes get`; if it fails, stop and say why.
3. **Run.**
   - rename / duplicate / upgrade: say in one line what will happen, then call.
   - delete / publish: `--dry-run` → restate (theme name + id; "permanent" for delete; for
     publish, which theme it replaces — read it with `themes list --params '{"published":"1"}'`)
     → wait for the user's explicit go-ahead in a later turn → call.
4. **Verify** with the re-read in the table below, and report what it shows.

## Pre-checks and facts

| Command | Pre-check (stop if it fails) | Facts to state |
|---|---|---|
| rename | New name equals the current `name` → nothing to change, no call | Changes only the display name; content, published state and id stay. Send the name exactly as given — no local length check, no truncation, no escaping beyond valid JSON |
| duplicate | — | New theme, new id, `published:"0"`; the source is untouched. No name given → `<source name> copy`, and say so |
| upgrade | Read `c_version`, `newest_c_version`, `has_newest_version`; missing → no call (themes uploaded from local development can't be upgraded); `has_newest_version` not true → "already on the newest version (<c_version>)", no call | Creates a NEW theme at `newest_c_version` carrying over the configuration; the original is unchanged and stays live if it was. `name` optional — omitted keeps the current name |
| delete | `published:"1"` or `default:"1"` → can't be deleted: "publish another theme first, then delete this one" | Permanent — there is no undo |
| publish | Already `published:"1"` → it is already live, no call | Replaces the live theme for buyers immediately. Changes still in an unsaved edit session are not included — save them first, or use `+edit … --promote --publish` ([card-edit.md](card-edit.md)) |

Duplicate = a same-version clone; upgrade = a new theme on the newest version. Renaming a file
inside a theme is a file operation ([files-versions.md](files-versions.md)), not a theme rename.

## Verify after the write

| Command | Re-read |
|---|---|
| rename | `themes get` → `.data.theme.name` |
| duplicate | `themes list --params '{"page_size":250}'` → the row with the new name that wasn't there before → its `id` |
| upgrade | `themes get` on the returned id |
| delete | `themes get` → 404 `Record not found` |
| publish | `themes list --params '{"published":"1"}'` |

Duplicate and upgrade never publish. Everything after them (edits, preview, publish) uses the
NEW id — pass `-t <new id>`, or later calls land on another theme.

## Errors & recovery

| Error | Meaning | Do |
|---|---|---|
| duplicate → `ServerError` 500 `internal server error` | The copy (theme, files, template bindings) is written before the step that fails, so it usually exists | Never retry blindly: `themes list`, look for the new name; retry only if it is absent |
| upgrade → 422 `version limit` | This theme can't be upgraded through this call (seen on themes several major versions behind) | Tell the user; don't retry |
| upgrade → 422 `version newest` | Already on the newest version | Say so |
| rename rejected | A server-side name rule | Relay the message and ask for another name; never shorten it yourself |
| delete refused | The theme is published / default | Say so; another theme must be published first (its own consent) |
| `Record not found` (404) | Wrong or already-deleted id | Re-list and re-resolve |

## Rules

- Several themes in one request → one call per theme, reporting each result. For delete,
  restate the full list (name + id each) and get consent for exactly that list.
- A name that matches no theme → "no theme named <X>" plus the existing theme names; no write.
  If the name looks like a product, collection or discount, ask whether that was meant — don't
  switch to another domain on your own.
- After any failed write, re-read before trying again.

## Output

- rename: old name → new name, as the re-read shows.
- duplicate / upgrade: new theme name and id, "unpublished — the live theme hasn't changed";
  upgrade adds "<c_version> → <newest_c_version>, the original is kept".
- delete: "<name> was permanently deleted".
- publish: "<name> is now live". Name the replaced theme — from `revoke_publish_id` (look it up
  with `themes get`) or from the live theme read before publishing — and add "to roll back,
  publish it again". `revoke_publish_id` is not always returned; never guess it.
