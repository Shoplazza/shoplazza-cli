# ljs-countdown — time left until an event

Time left until an activity starts or ends; limited-time offer / flash-sale countdowns. lessjs
reference: [spz-countdown](https://lessjs.shoplazza.com/latest/components/spz-countdown/) (write
the tag as `ljs-countdown`).

## Rules

1. Use `ljs-countdown`; don't write your own countdown JS.
2. Pick exactly one time source (per the business need); don't mix them:
   - `timeleft-seconds`: seconds remaining (common for banners: days × 86400).
   - `end-date`: a fixed end time. Pair it with a `date_picker` setting so the merchant picks it;
     its format and preset value must both be `YYYY-MM-DD HH:mm:ss` (the schema side of
     `date_picker` is in [schema-rules.md](../schema-rules.md) → "Full field reference"). The
     component uses the visitor's browser time zone, not the store's.
   - `timestamp-seconds`: the end time as a Unix timestamp in seconds.
   - When the time comes from a setting it may be empty; with all three sources empty the
     component errors and no countdown appears. Wrap `<ljs-countdown>` in
     `{% if <that setting> != blank %}`, or fall back to `timeleft-seconds` when it's empty.
3. If the settings are in days / hours, convert them to seconds in Liquid before passing them on.
4. `loop`: restart the countdown when it ends (boolean, no value), as needed.
5. The digits' markup goes in an inline `<template>` inside the component. Template variables:
   `${d}` `${h}` `${m}` `${s}` (days / hours / minutes / seconds, no zero padding), `${dd}` `${hh}`
   `${mm}` `${ss}` (same, zero-padded to two digits), `${SSS}` (milliseconds, three digits). The
   template itself follows [template.md](template.md).
6. Marketing banner: title / button in plain Liquid; only the digits use the countdown.
7. `layout`: see the single-value table in [writing.md](writing.md) R4.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `timeleft-seconds` | Seconds remaining (end = now + this) | Yes, unless `timestamp-seconds` or `end-date` is set | Common for banners |
| `end-date` | End date | Yes, unless `timeleft-seconds` or `timestamp-seconds` is set | Formats such as `YYYY-MM-DD HH:mm:ss` or `YYYY/MM/DD HH:mm:ss` |
| `timestamp-seconds` | End as a timestamp in seconds | Yes, unless `timeleft-seconds` or `end-date` is set | As needed |
| `loop` | Loop | No | Boolean, no value. Without `offset-seconds` it only loops with `timeleft-seconds` |
| `offset-seconds` | Seconds to keep counting after the end | No | Only takes effect with `loop` |
| `manual` | Start only when `restart` is called | No | Rarely used |

## Actions & events

- Action: `restart` (start the countdown again; no parameters).
- Event: `timeout` (the countdown reached zero).

## Skeleton (inline template, preferred)

```liquid
{% assign duration_days = block.settings.countdown_duration | default: 3 %}
{% assign duration_seconds = duration_days | times: 86400 %}
{% capture countdown_id %}countdown-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  {% if block.settings.title != blank %}
    <h2>{{ block.settings.title }}</h2>
  {% endif %}

  <ljs-countdown
    id="{{ countdown_id }}"
    layout="container"
    timeleft-seconds="{{ duration_seconds }}"
    {% if block.settings.enable_loop %}loop{% endif %}
  >
    <template>
      <div class="cd-timer">
        <span class="cd-num">${d}</span><span class="cd-sep">:</span>
        <span class="cd-num">${h}</span><span class="cd-sep">:</span>
        <span class="cd-num">${m}</span><span class="cd-sep">:</span>
        <span class="cd-num">${s}</span>
      </div>
    </template>
  </ljs-countdown>

  {% if block.settings.button_text != blank %}
    <a href="{{ block.settings.button_link.url | default: '#' }}">{{ block.settings.button_text }}</a>
  {% endif %}
</div>
```
