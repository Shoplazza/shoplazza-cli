# ljs-vimeo — Vimeo embed

Embeds an external Vimeo video. Keep it strictly apart from [ljs-video](ljs-video.md) (store-uploaded
mp4/hls); never mix them.

## Rules

1. External Vimeo → `ljs-vimeo`; the store's own / CDN mp4 or hls → `ljs-video`.
2. `videoid` is required: the numeric id as a string (e.g. `246545368` for
   `https://vimeo.com/246545368`).
3. If the schema accepts a full Vimeo URL, strip the query in Liquid and take the last path segment
   as the bare `videoid`; a numeric input is used as is. With no value, output a placeholder — never
   render an empty `<ljs-vimeo>`.
4. `layout` supports `responsive` / `intrinsic` / `fixed` / `fixed-height` / `fill` / `flex-item`;
   usually `responsive` with `width="1444"` `height="504"`.
5. The component's only attributes are the required `videoid` and the optional `referrerpolicy`.
   `src` / `autoplay` / `loop` are not recognised — this component can't autoplay, so don't write
   them for show.
6. No actions, no events; there is no playback-control API.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `videoid` | Vimeo video id | yes | numeric string |
| `layout` | Layout | no | usually `responsive` / `fixed` |
| `width` / `height` | Size | no | usually `1444` / `504` |
| `referrerpolicy` | iframe referrer policy | no | optional string |

## Skeleton

```liquid
{% assign videoid = block.settings.vimeo_id %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  {% if videoid != blank %}
    <ljs-vimeo
      videoid="{{ videoid }}"
      layout="responsive"
      width="1444"
      height="504"
      {% if block.settings.referrer_policy != blank %}referrerpolicy="{{ block.settings.referrer_policy }}"{% endif %}
    ></ljs-vimeo>
  {% else %}
    <div class="media-placeholder" aria-hidden="true"></div>
  {% endif %}
</div>
```
