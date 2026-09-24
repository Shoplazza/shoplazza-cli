# themes files & versions — theme source files and their change history

A theme is a set of source files grouped by kind: `templates` (one per page; stored as JSON page
configs even though the names end in `.liquid`), `sections` (card code — a theme card's `type`
equals its file name without `.liquid`), `snippets`, `assets` (CSS / JS), `locales` (translation
strings), `config` (`settings_schema.json`, `settings_data.json`, …) and `layout`. `themes file …`
reads and writes them; `themes version …` reads each file's change history.

**File writes skip the edit session**: no draft, no preview, no validation. On the published theme
a write is live on the storefront the moment it returns.

## Which tool

| Goal | Use | Not |
|---|---|---|
| Change what a page shows: cards, their settings, order, visibility | `+page` → `+edit` ([card-edit.md](card-edit.md)) | Rewriting `templates/<page>.liquid` |
| Theme-wide settings (colors, fonts, …) | `session get-config` / `update-config` ([global-config.md](global-config.md)) | Editing `settings_data.json` |
| An AI card's source | `block +get --with-content` / `block +edit` ([block/block-cli.md](block/block-cli.md)) | `file get` with type `blocks` (session AI cards are not theme files) |
| Read a card's liquid, a snippet, CSS / JS, a locale file | `file tree` → `file get` | — |
| Change code or assets the editor can't reach (custom CSS, a locale string, section liquid) | `file get` → edit locally → `file update` | — |
| When did a file change | `version list` / `version records` | — |

Template files don't track the editor: a custom template made with `template create` never shows
in `file tree`. Read page content with `+page`.

## Commands

| Intent | Command | Returns |
|---|---|---|
| Files by group | `themes file tree --params '{"theme_id":"<id>"}'` | `.data.{assets,configs,layouts,locales,sections,snippets,templates}[]` = `{id, location}` |
| Read one file | `themes file get --params '{"theme_id":"<id>","type":"sections","location":"<name>.liquid"}'` | `.data.theme_file.{id, type, location, content}` |
| Create | `themes file create --params '{"theme_id":"<id>"}' --data '{"doc":{"type":"assets","location":"<name>.css","content":"<text>"}}'` | `{}` |
| Replace the whole content | `themes file update --params '{"theme_id":"<id>"}' --data '{"doc":{"type":"<type>","location":"<name>","content":"<full new content>"}}'` | `{}` |
| Rename | `themes file rename --params '{"theme_id":"<id>"}' --data '{"doc":{"type":"<type>","location":"<old>","new_location":"<new>"}}'` | `{}` |
| Delete (no `doc` wrapper) | `themes file delete --params '{"theme_id":"<id>"}' --data '{"type":"<type>","location":"<name>"}'` | `{}` |
| One file's history | `themes version list --params '{"theme_id":"<id>","type":"<type>","location":"<name>"}'` | `.data.versions[]` = `{id, version, created_at}`, newest first |
| Every file's history | `themes version records --params '{"theme_id":"<id>"}'` | `.data.versions[]` = `{id, version, type, location, created_at}` |
| One version's metadata | `themes version get --params '{"theme_id":"<id>","version_id":"<version_id>"}'` | `.data.version.{id, version, type, location, created_at, updated_at}` — no content |

`location` is the name inside its group (`theme.liquid`, `index.liquid`, `theme.css`), with no
folder prefix. Version lists carry no pagination fields; the whole list comes back at once.

### `type` values — tree group ≠ file type

| `file tree` group | `type` in file / version calls |
|---|---|
| `layouts` | `layout` |
| `configs` | `config` |
| `templates` · `sections` · `snippets` · `assets` · `locales` | the same word |
| — (never listed in the tree) | `blocks` (`version list` rejects it) |

Always pass both `type` and `location`: `file get` without them reads `layout` / `theme.liquid`,
and `file delete` without `location` falls back to `assets/a.js` and deletes that file.

## Safety

- Is the theme live? `themes get --params '{"theme_id":"<id>"}' --jq '.data.theme.published'` →
  `"1"` = published.
- **Published theme**: every `file create|update|rename|delete` is publish-class — `--dry-run` →
  restate (file, what changes, "goes live immediately, there is no draft or undo") → wait for the
  user's explicit go-ahead in a later turn.
- **Unpublished theme**: create / update / rename run directly; `file delete` still needs consent.
- `assets/theme.css` and the theme's main layout file can't be deleted.
- Deleting a `sections` file that a page still uses does not fail — that card silently stops
  rendering. Before deleting one, check `+page` rows on the pages that could use it (row `type` =
  file name without `.liquid`) and say which pages would lose the card.
  Treat renaming a used section file the same way.

## Rules

1. `file update` replaces the whole file. `file get` first, change only the part asked for, and
   send the complete result — never a fragment.
2. Check existence with `file tree` before `get` / `delete`: both answer ok `{}` for a missing file.
3. Writes return `{}`. Re-read with `file get` and report what the re-read shows.
4. Never change page content or theme settings by rewriting template JSON or `settings_data.json`
   (see Which tool): it bypasses the session, the schema checks and the preview.
5. **Restoring an old version is not possible through the CLI.** `version get` returns metadata
   only, and there is no restore command. Say so. If the user supplies the old content, `file
   update` can write it back under the same safety rules as any write.

## Errors & quirks

| Symptom | Meaning → fix |
|---|---|
| `get` / `delete` → `{"ok":true,"data":{}}` | The file doesn't exist (no 404) — check `file tree` |
| `create` → 422 `InvalidParameter`, `message:"文件已存在"` | The file already exists → `file update` |
| 400 `invalid GetThemeFileRequest.Type: value must be in list […]` | A tree group name was used as `type` (`layouts`, `configs`) → type table |
| `version list` → `data:{}` | No history: `create` makes no version (the first `update` is version 1), or the file was renamed — its history stays under the old `location` |
| After `rename`, the old name reads `{}` | Expected; the file id is kept under the new name |

## Output

Name the file as `<type>/<location>`, what changed (re-read), and whether it is already live
(published theme). For history, a table of `version` and `created_at`, newest first.
