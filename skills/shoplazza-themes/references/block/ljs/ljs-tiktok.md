# ljs-tiktok — TikTok embed

Embeds an external TikTok video. Videos uploaded to the store (mp4/hls) use
[ljs-video](ljs-video.md); never mix the two.

## Rules

1. Use it only when the request explicitly asks for a TikTok video. External TikTok → this
   component; the store's own uploads → `ljs-video` (`mp4`/`hls`). Never hand-write a TikTok iframe.
2. Accept only the full canonical URL, shaped `https://www.tiktok.com/@user/video/<videoid>`. Don't
   take a bare id and build `https://www.tiktok.com/video/<id>` — that address fails TikTok's
   oEmbed lookup.
3. Write both `videoid` and `src`; missing either makes the component error. Parse the numeric last
   path segment of the full URL as `videoid`, and pass the full URL unchanged as `src`. `src` is used
   to fetch the cover (oEmbed); `videoid` builds the player.
4. `layout` is a sizing layout: `fill` / `fixed` / `fixed-height` / `flex-item` / `intrinsic` /
   `responsive`; for portrait video usually `responsive` with `width`/`height`.
5. `autoplay` is a configurable boolean: provide an `enable_autoplay` checkbox; when on, output
   `autoplay` with no value; when off, omit it.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `videoid` | TikTok video id | yes | value attribute |
| `src` | TikTok video page URL | yes | used for the oEmbed cover |
| `layout` | Layout | no | rule 4 |
| `width` / `height` | Size | no | usually needed with `responsive` / `fixed` |
| `autoplay` | Play once loaded | no | boolean, no value; omit when off |

No events, no actions. Use only the attributes in the table.

## Skeleton

```liquid
{% assign tiktok_src = block.settings.tiktok_url %}
{% assign tiktok_parts = tiktok_src | split: '/' %}
{% assign tiktok_last = tiktok_parts | last | split: '?' | first %}
{% assign tiktok_id = tiktok_last | strip %}
{% assign valid_tiktok_url = false %}
{% if tiktok_src contains 'https://www.tiktok.com/' and tiktok_src contains '/video/' %}
  {% assign valid_tiktok_url = true %}
{% endif %}

<div
  class="{{ root_cls }}"
  {{ block.shoplaza_attributes }}
>
  {% if valid_tiktok_url and tiktok_id != blank %}
    <ljs-tiktok
      layout="responsive"
      width="{{ block.settings.video_width | default: 325 }}"
      height="{{ block.settings.video_height | default: 578 }}"
      videoid="{{ tiktok_id }}"
      src="{{ tiktok_src }}"
      {% if block.settings.enable_autoplay %}autoplay{% endif %}
    ></ljs-tiktok>
  {% else %}
    <div class="media-placeholder" aria-hidden="true"></div>
  {% endif %}
</div>
```
