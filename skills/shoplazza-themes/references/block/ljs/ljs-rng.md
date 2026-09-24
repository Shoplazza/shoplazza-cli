# ljs-rng — random number

A number that jumps around between `min` and `max`. Never use it to fake social proof such as
"X people are viewing" or "only N left".

## Rules

1. Use `ljs-rng`. No `setInterval` + `Math.random` of your own.
2. Write all four attributes: `min`, `max`, `change-range`, `interval-seconds`. The component does
   nothing when `min` ≥ `max` or `change-range` is `0`.
3. **Never** wrap the output in pressure copy ("viewing now", "low stock", "xxx sold").
4. No actions. One event: `change`, fired on each jump; payload `current` / `previous` / `change` /
   `min` / `max`.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `min` / `max` | Range | yes | write both |
| `change-range` | Size of each jump | yes | number |
| `interval-seconds` | Seconds between jumps | no, default `3` | number of seconds |

## Skeleton

```liquid
<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-rng
    layout="container"
    min="{{ block.settings.min | default: 20 }}"
    max="{{ block.settings.max | default: 100 }}"
    change-range="{{ block.settings.change_range | default: 8 }}"
    interval-seconds="{{ block.settings.interval_seconds | default: 2 }}"
  ></ljs-rng>
</div>
```
