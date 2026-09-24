# ljs-timeago — relative time

Shows an absolute time as "how long ago".

## Rules

1. Use `ljs-timeago`. No hand-written relative-time JS.
2. `datetime` is required and is the only attribute to write.
3. `datetime` takes a parseable date string: `YYYY/MM/DD`, `YYYY/MM/DD HH:mm`, or
   `YYYY/MM/DD HH:mm:ss` (e.g. `2021/5/10 15:36:11`). When it comes from settings, output it as is;
   don't concatenate other copy into it.
4. No actions, no events.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `datetime` | Absolute time | yes | value attribute |

## Skeleton

```liquid
<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  {% if block.settings.label != blank %}
    <span class="timeago-label">{{ block.settings.label }}</span>
  {% endif %}
  <ljs-timeago
    layout="container"
    datetime="{{ block.settings.datetime | escape }}"
  ></ljs-timeago>
</div>
```
