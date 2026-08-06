# orders risks — fraud risk records

Risk signals on a single order: the platform's automated fraud assessment plus custom risk
records a merchant (or app) attaches after manual review — e.g. "标记成高风险".

## Commands

| Intent | Command |
|---|---|
| View an order's risk signals | `orders risks list --params '{"order_id":"<id>"}'` → `.data.assessments[]` (platform's automated labels) + `.data.infos[]` (record entries) |
| Get one risk record | `orders risks get --params '{"order_id":"<id>","id":"<risk-id>"}'` |
| Flag an order (write → restate or `--dry-run` first) | `orders risks create --params '{"order_id":"<id>"}' --data '{"risk":{…}}'` → `.data.risk` |
| Edit a record | `orders risks update --params '{"order_id":"<id>","id":"<risk-id>"}' --data '{"risk":{…}}'` |
| Remove a record (destructive → `--dry-run` first) | `orders risks delete --params '{"order_id":"<id>","id":"<risk-id>"}'` |

## Body (`schema orders.risks.create` / `.update`)

```json
{"risk": {
  "level": "high",
  "details": ["IP 和收货地址不一致"],
  "properties": {"key": "value"}
}}
```

- `level` — **required on create**; enum `low` · `medium` · `high` (高风险 → `high`).
- `details` — string array; put the user's stated reasons here, verbatim.
- `properties` — optional string→string custom map.
- Everything nests under the required `risk` object.
- Deleting/updating a record leaves the order and its other risk records untouched.
