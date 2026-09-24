# themes block delete — delete an AI card's source file

Delete an AI card's liquid file; every instance of it in the theme disappears with it. Use this
only when the user explicitly asks to delete the card's source, or the card itself from the theme.
Command details: [block-cli.md](block-cli.md).

## Scope

- **Only AI cards** — the instance `type` starts with `blocks/gen_`. Theme cards and blocks that
  ship with the theme have no source that can be deleted: say so. They can only be removed from
  the page ([card-edit.md](../card-edit.md)).
- **Unsure whether to delete the source or just take the card off the page?** Take it off the page
  ([card-edit.md](../card-edit.md)) and say why: a card removed from a page can be put back, a
  deleted source can't.

## Flow

1. **Identify** the card → `gen_id` ([page-read.md](../page-read.md)): the `type` of its block row
   without `blocks/`.
2. **Read the impact first:**
   `themes block +get -t <theme_id> --session <oseid> --id <gen_id>` (no `--section`) →
   `ref_count` and `instances`. This is the only way to know the impact: `delete-gen` doesn't look
   at references and deletes however many places use the card.
3. **Dry-run, restate, wait** (shoplazza-common → Safety protocol):

   ```bash
   themes block delete-gen --params '{"oseid":"<oseid>","type":"blocks/<gen_id>"}' --dry-run
   ```

   Restate per Rules below, then stop and end the turn.
4. **Run it** in a later turn, only after the user explicitly agrees — the same command without
   `--dry-run`.
5. **Report** (Output).

## Rules

`ref_count` decides what the restatement says:

| `ref_count` | Say |
|---|---|
| 0 | The card isn't on any page yet; only its source file goes. |
| 1 | Which page it's on; it disappears from there. |
| ≥ 2 | How many places, listing each page from `instances`; all of them disappear at once. |

Each `instances` row is one placement: `template` is the page, `target` its position on the page.

Restate three things, in words the user understands: the card's name (its `cname` from the page
read), the places that will lose it, and that it's permanent — the card can't be put back on a
page afterwards. Name pages by what they are (`index` is the homepage, `product` the product
detail page); never show `ref_count`, `template`, `target` or their raw values.

## Undo

The response carries a `revert_id`. `themes block revert-gen` with it, in the same session, brings
the card back — its file, the `_blocks` container it sat in, and its settings:

```bash
themes block revert-gen --params '{"oseid":"<oseid>"}' --data '{"revert_id":"<revert_id>"}'
```

Use it only when the user asks to undo; don't offer it as a safety net in the restatement.

## Output

- Success is the command returning without error; summarize from the list read in step 2: which
  card was deleted and which places lost it.
- Preview link: `themes +preview -t <theme_id> --oseid <oseid>`.
- The deletion sits in the edit-session draft; buyers still see the original pages until it's
  saved and published (SKILL.md → Workflow).
