# themes block generate — write a new AI card and place it on a page

You write the new card's liquid source yourself (markup, style and `{% schema %}` in one file),
check it by hand, then write it into the theme and place it on the page with one
`themes block +edit`. The only thing this creates is one new block file, plus the container
section that holds it on the page. Command details: [block-cli.md](block-cli.md).

## When to generate

Generate only when one of these holds:

- **No addable card can do it.** The theme's addable cards (`themes section cards`, their fields
  via `themes +card-schema`; see [card-add.md](../card-add.md)) have no card for the function or
  effect, none can produce the layout, or the matching card lacks a field the user named.
- **The user explicitly asks** to generate or customize a card (生成一个卡片 / 自定义卡片 /
  generate a card).

When one holds, go ahead; don't ask whether to generate. Changing values on an existing card is
[card-edit.md](../card-edit.md); changing an existing AI card's layout or structure is
[edit-block.md](edit-block.md).

## Before writing

These lookups don't depend on the source; run them while you read the rules.

| Value | Where it comes from |
|---|---|
| `theme_id` | SKILL.md → Rules for every operation (1) |
| `oseid` | The task's session; else `themes +page -t <theme_id> --template <template>` → `.data.oseid` |
| `template` | The page the card goes on ([page-read.md](../page-read.md)); merchant-created pages → [block-cli.md](block-cli.md) → Values to gather |
| Card copy language | The primary market's default language (below) |

```bash
shop markets list --jq '.data.markets[] | select(.is_primary) | .id'
shop markets list-language --params '{"id":"<market_id>"}' --jq '.data.languages[] | select(.is_default) | .code'
```

- Text the user dictated is written verbatim, and the card language then follows that text even
  if it differs from the storefront language; write the rest of the copy to match it.
- All of the card's copy (defaults and preset values) is in that one language. Schema `label` /
  `info` / `cname` are bilingual objects regardless ([schema-rules.md](schema-rules.md)).

Pin the requirement down before writing: the need itself plus the form, content, count and
trade-offs already settled in the conversation. For a product-list card, don't shrink the product
card to "image, title, price, click through to the product page": quick add (including the
in-card variant panel for multi-option products) is a default element even when nobody mentioned
it. Leave it out only when the merchant says they don't want quick add.

## Reading list

Read all of these, in parallel where the host allows:

| File | What it covers |
|---|---|
| [concepts.md](concepts.md) | The terms: block, inline sub-block, schema, ljs component, AI card, a card's names |
| [liquid-rules.md](liquid-rules.md) | Shoplazza Liquid vs Shopify, and the platform's writing rules |
| [schema-rules.md](schema-rules.md) | Every `{% schema %}` rule: structure, control choice, full field reference |
| [mobile-rules.md](mobile-rules.md) | The mobile pitfalls that break the phone page |
| [ljs/selection.md](ljs/selection.md) | Which ljs components the card needs (intent → component) |
| [ljs/writing.md](ljs/writing.md) | Rules every ljs component follows |
| [kind-and-components.md](kind-and-components.md) | How to settle components and card kind, and what else that makes you read |
| [self-check.md](self-check.md) | The checklist you run over the finished file |

Then settle components, kind and objects per [kind-and-components.md](kind-and-components.md)
and read every file it names (component docs, [ljs/template.md](ljs/template.md), the kind file,
[objects.md](objects.md)) before writing.

If the host can run sub-agents, steps 1–3 of the flow may be delegated to one with this same
reading list; have it bring back only the file path and a short summary (form, settings,
components used).

## Flow

1. **Write the whole file in one go** to a local working file (e.g. `./card.liquid`; the name is
   yours — the server assigns `gen_<id>`): the requirement comment at the top, the liquid/HTML and
   `<style>`, then one complete `{% schema %}` at the end.
   - The body follows [liquid-rules.md](liquid-rules.md) and [mobile-rules.md](mobile-rules.md);
     ljs components follow [ljs/writing.md](ljs/writing.md) and each component's own doc.
   - The schema follows [schema-rules.md](schema-rules.md): root setting vs inline sub-block,
     control choice, field-by-field shape. Every `block.settings.<id>` read matches a declared id.
2. **Self-check:** go through [self-check.md](self-check.md) over the whole file.
3. **Fix every hit** with exact edits (as in [edit-discipline.md](edit-discipline.md)), then run
   the check again:
   - Schema, preset and read-but-undeclared hits → fix the `{% schema %}` declarations.
   - Everything else, including declared-but-unused settings → fix the body (actually render
     the setting).
   - The same hit twice in a row → re-read the file before fixing again. The file on disk may
     differ from what you remember (an earlier edit may have changed something else); don't
     rewrite from memory.
   - After 3 fix rounds, stop: don't write to the theme; see Errors & recovery.
4. **Write and place** in one call:

   ```bash
   themes block +edit -t <theme_id> --session <oseid> --template <template> \
     --content ./card.liquid --section-name '{"zh-CN":"<name>","en-US":"<name>"}'
   ```

   - Without `--target` the CLI adds a new container at the end of the page and places the card
     in it. Pass `--target <sid>.blocks` only when the user wants the card inside a specific
     existing section.
   - `--section-name` is what merchants see for that container in the editor's structure tree;
     without it they see `_blocks`.
   - From here on, the card is the returned `type` / `doc.id`; the local file name means nothing to
     the server. The next edit of this card fetches its source again
     ([edit-block.md](edit-block.md)) instead of reusing the local file.
5. **Re-read:** `themes block +get -t <theme_id> --session <oseid> --id <doc.id> --section <instance.target>`
   → confirm the instance and its `settings`.
6. **Report** (Output) and stop at the preview.

## Errors & recovery

| Situation | Do |
|---|---|
| Self-check still failing after 3 rounds | Don't write. Find the closest addable cards ([card-add.md](../card-add.md)); tell the user why generation failed and suggest up to 3 of them (name + one line on how each differs). Let the user choose one of them or drop it. Never force a card in or pass one off as what they asked for. |
| Error with `stage:"write"` | The server rejected the file (a liquid parse error, invalid or missing `{% schema %}`); nothing was written. Fix the file and send the same command again. |
| `stage:"place"` with `reverted:true` | Rolled back; send the same command again. |
| `stage:"place"` with `revert_failed:true` | Don't resend blindly. The hint carries a `themes block revert-gen` command → run it, then retry once the failed op (`results` / `failed`) is fixed; no such command in the hint → report the failure with `block_type` and `revert_id` (details: [block-cli.md](block-cli.md)). |
| `degraded` in the output | The card is placed; the listed extras (`section_name`, `ops`) didn't land. Redo them with `themes +edit` ([card-edit.md](../card-edit.md)) or tell the user. |
| Page is `order` / `order_verify` | Not supported (those pages can't be previewed); say so and send nothing. |

Every other error shape: [block-cli.md](block-cli.md) → Errors & recovery.

## Output

- Give the `preview_url` from the response verbatim, or run
  `themes +preview -t <theme_id> --oseid <oseid> [--path /products/<handle>]`. For a card on a
  `customers/*` page, say the preview opens a login page first: log in as a customer and refresh.
- Summarize what the card shows and which settings the merchant can adjust, named by their
  `label` in the user's language — never by setting id.
- The card sits in the edit-session draft; buyers can't see it yet. Saving and publishing:
  SKILL.md → Workflow.
