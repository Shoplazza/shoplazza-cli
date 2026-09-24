# ljs-youtube — YouTube embed

Embeds an external YouTube video. Keep it strictly apart from [ljs-video](ljs-video.md)
(store-uploaded mp4/hls); never mix them.

## Rules

1. External YouTube → `ljs-youtube`; the store's own / CDN mp4 or hls → still `ljs-video`. Never use
   this component for store video, and never write a bare `<iframe src="youtube...">`.
2. `videoid` is required: the video id string (e.g. `Wv6Up5YSi5s`), not the full watch URL.
3. If the schema accepts a full URL, normalise it to a bare `videoid` in Liquid first: handle
   `youtu.be/<id>` and `watch?v=<id>`, and cut any trailing `&` / `?` / `/` part. Never put a full
   URL into `videoid`.
4. `layout`: `responsive` / `fixed` / `fill`; usually `responsive` with `width` / `height`.
5. Provide two checkboxes, `enable_autoplay` and `enable_loop`, controlling the valueless boolean
   attributes `autoplay` and `loop`; omit each attribute when off. With `loop`, also output
   `data-param-playlist="{{ videoid }}"` — the public docs pair the two for looping.
6. No actions, no events; methods like `play()` don't exist.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `videoid` | YouTube video id | yes | string |
| `layout` | Layout | no | usually `responsive` / `fixed` |
| `width` / `height` | Size | no | give them with the layout |
| `autoplay` / `loop` | Autoplay / loop | no | boolean, no value |
| `data-param-playlist` | Goes with `loop` | no | same value as `videoid` |

Use only the attributes in the table: don't write `referrerpolicy`, `aria-label`, or any other
`data-param-*`, and don't pass a full URL through `src`.

## Skeleton

```liquid
{% assign videoid = block.settings.youtube_id %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  {% if videoid != blank %}
    <ljs-youtube
      videoid="{{ videoid }}"
      layout="responsive"
      width="{{ block.settings.width | default: 1280 }}"
      height="{{ block.settings.height | default: 720 }}"
      {% if block.settings.enable_autoplay %}autoplay{% endif %}
      {% if block.settings.enable_loop %}loop data-param-playlist="{{ videoid }}"{% endif %}
    ></ljs-youtube>
  {% else %}
    <div class="media-placeholder" aria-hidden="true"></div>
  {% endif %}
</div>
```
