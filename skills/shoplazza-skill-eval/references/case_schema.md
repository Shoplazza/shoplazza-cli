# Case schema — authoritative field definitions

Cases live in `cases/<domain>/<operation>.json` (behavior + per-domain routing) and
`cases/_routing/collisions.json` (mandatory cross-domain routing). Each file holds a
**JSON array of case objects**. Two case types share this one schema; `type`
distinguishes them. Spec authority: `docs/skill/eval-system-design.md` §3.

## Common fields (both types)

| Field | Req | Type | Meaning |
|---|---|---|---|
| `case_id` | yes | string | Unique across the whole suite. Convention: `<domain>.<operation>-NN` for behavior, `collision.<name>` / `routing.<domain>-NN` for routing. Dedup key at load time. |
| `type` | yes | `"behavior"` \| `"routing"` | Which harness path runs it (see §Modes below). |
| `skill` | yes | string | The skill under test (behavior) / the **correct** target skill (routing). |
| `operation` | behavior: yes; routing: optional | string | `<domain>.<operation>`, e.g. `discounts.rebate`. Selects the per-operation rubric add-on (create/update/delete/query). |
| `scenario` | yes | string | One-line human description of what the case probes (bilingual OK). |
| `input` | yes | string | The user utterance handed verbatim to the agent-under-test. |
| `expected` | yes | object | Assertions — see below. The agent-under-test must NEVER see this object. |
| `notes` | no | string | Authoring context, known caveats. |

## `expected` — behavior cases

| Field | Req | Type | Checked by | Meaning |
|---|---|---|---|---|
| `route_to_skill` | yes | string | L1 | Skill the agent must state/use (usually equals `skill`). |
| `access_tier` | yes | `"shortcut"` \| `"leaf"` \| `"api"` | L1 + R-2 | Tier of the expected command. Tier is inferred from the emitted command: `+<verb>` → shortcut, `api rest` → api, else leaf. |
| `command_prefix` | yes* | string | L1 | The emitted command must start with this after stripping an optional `shoplazza` prefix, e.g. `discounts +rebate`. *Optional when `must_ask: true` (the right answer is a question, not a command). |
| `flags_must_contain` | no | string[] | L1 | Each entry must appear in the command string. An entry may pin a value (`"--target order"`) or just the flag name (`"--tiers"`). Whitespace-normalized substring match; `--flag value` and `--flag=value` both accepted. |
| `flags_must_not_contain` | no | string[] | L1 | None may appear. Use for defaulted flags the agent must not set or ask about (e.g. `--limit-max` when the user said nothing about caps). |
| `params_must_contain` | no | string[] | L1 | Substrings that must appear inside the `--params` / `--data` JSON payloads (for leaf ops), e.g. `"\"ids\":["`. |
| `must_ask` | yes | boolean | L1 + R-5 | `true` → the agent SHOULD ask (a required no-default field is missing) and must NOT emit a write command. `false` → the agent must NOT ask. |
| `ask_must_cover` | no | string[] | L1 + R-5 | Only with `must_ask: true`: keywords the clarifying question must mention (e.g. `["target", "tiers"]`). Any-of match per entry (`"a|b"` = either). |
| `semantic_rubric` | no | string[] | L2 | Per-case pass/fail statements the judge scores individually (each P/F). Ground every statement in the user's own words + verified CLI behavior. |

## `expected` — routing cases

| Field | Req | Type | Meaning |
|---|---|---|---|
| `route_to_skill` | yes | string | The correct skill. **If this skill is not yet built** (not present in the visible description set), the check degrades: pass = the agent routes to NO listed skill (states `NONE` or equivalent) — see "Unbuilt targets" below. |
| `must_not_route_to` | no | string[] | Routing to any of these = hard FAIL (R-1, P0). |
| `semantic_rubric` | no | string[] | Optional judge statements (e.g. "states it would use `shop metafields-resource`"). |

Routing cases omit command detail — only the trigger is asserted. The emitted command's
service prefix (if the agent volunteers one) corroborates the stated skill; disagreement
between stated skill and command prefix is itself a finding (spec §7).

### Unbuilt targets (early waves)

`cases/_routing/collisions.json` names skills from later waves (`shoplazza-shop`,
`shoplazza-products`, `shoplazza-orders`). Until those skills exist, the runner only
enforces `must_not_route_to` plus "did not claim an unlisted skill exists"; the
`route_to_skill` hit is recorded as `skipped_unbuilt`. When the wave lands, the same
case automatically starts enforcing the full assertion — do not fork the case.

## Authoring rules

1. **Ground `expected` in the real binary.** Verify every command/flag with
   `shoplazza <svc> --help` / `<cmd> --help` / `schema <svc>.<cmd>` before writing it.
   A case asserting a nonexistent flag poisons the whole suite (and L0 will not save
   you — it lints skills, not cases).
2. Values in `flags_must_contain` must be traceable to `input` — the fabricated-value
   scan (L1) cross-checks numbers in the command against numbers in the utterance.
3. One intent per case. Multi-step flows (get → cancel) belong in ONE case whose
   `command_prefix` targets the final write and whose `semantic_rubric` asserts the
   preceding steps.
4. `must_ask: true` cases: put the missing fields in `ask_must_cover`; add a
   `flags_must_not_contain` guard against the agent fabricating those same values.
5. Keep utterances realistic and bilingual across the suite (both 中文 and English
   inputs — routing keywords must fire for both).
