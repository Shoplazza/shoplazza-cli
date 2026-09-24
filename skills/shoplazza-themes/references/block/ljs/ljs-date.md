# ljs-date — formatted dates

For showing a date/time in a given `format`. lessjs reference:
[spz-date](https://lessjs.shoplazza.com/latest/components/spz-date/) (write the tag as `ljs-date`).

## Rules

1. Use `ljs-date`; don't pull in a third-party date library for formatting.
2. The time goes in the `datetime` attribute, not as child text.
3. Custom display uses `format` (e.g. `DD MMM, YYYY HH:mm:ss`). Always provide a text setting
   `format` and output it on the component; never hard-code it. The component's default is
   `YYYY-MM-DD HH:mm:ss` (includes the time) — always pass `format` explicitly instead of relying
   on it.
4. The component accepts only `datetime` / `format` / `layout`; write no other names.
5. `datetime` is validated: a string that can't be parsed as a date makes the component error and
   the whole card fails to render. Make sure `datetime` is always a valid, parseable date string.
6. If the page offers a date input, give `ljs-date` a unique `id` and bind the input with
   `@change="<date_id>.toggleAttribute(key='datetime', force=true, value=event.value)"`. Changing
   the input updates the `datetime` attribute, which makes the component re-format. Never
   hand-write JS for this.
7. Default merchant-facing copy such as labels goes into `default` and the preset.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `datetime` | Date/time to format | Yes | The only place the time goes. Documented forms: `YYYY/MM/DD`, `YYYY/MM/DD HH:mm`, `YYYY/MM/DD HH:mm:ss`, or a number |
| `format` | Display format | No | e.g. `MMM DD YYYY, HH:mm` |

`format` placeholders: `YY` / `YYYY` (year), `M` / `MM` (month), `MMM` (short month name), `D` /
`DD` (day of month), `ddd` (short weekday), `H` / `HH` (24-hour), `h` / `hh` (12-hour), `m` / `mm`
(minutes), `s` / `ss` (seconds), `a` / `A` (am/pm, AM/PM).

Actions / events: `ljs-date` has none of its own; use the lessjs global `toggleAttribute` action
from an input to update its `datetime` (rule 6).

## Skeleton

```liquid
{% capture date_id %}date-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  {% if block.settings.label != blank %}
    <span class="date-label">{{ block.settings.label }}</span>
  {% endif %}

  <ljs-date
    id="{{ date_id }}"
    layout="container"
    datetime="{{ block.settings.datetime }}"
    format="{{ block.settings.format | default: 'YYYY-MM-DD HH:mm:ss' }}"
  ></ljs-date>

  <input
    type="date"
    value="{{ block.settings.datetime }}"
    @change="{{ date_id }}.toggleAttribute(key='datetime', force=true, value=event.value)"
  >
</div>
```

Keep this setting in both the schema and the preset:

```json
{
  "type": "text",
  "id": "format",
  "label": { "zh-CN": "日期格式", "en-US": "Date format" },
  "default": "YYYY-MM-DD HH:mm:ss"
}
```

The time always goes through `datetime`. Written as child text
(`<ljs-date>{{ block.settings.datetime }}</ljs-date>`) the component gets no value.
