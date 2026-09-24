# ljs-video — store-uploaded video

Plays a video uploaded to the store (mp4/hls).

## Rules

1. Store-uploaded video → `ljs-video`. External YouTube / Vimeo / TikTok →
   [ljs-youtube](ljs-youtube.md) / [ljs-vimeo](ljs-vimeo.md) / [ljs-tiktok](ljs-tiktok.md); don't
   push them into this component.
2. Put the sources on the `mp4` / `hls` attributes. HTML5 `<source>` children are ignored.
3. `poster`: cover image, usually `| img_url`. Required — without it the component errors.
4. Boolean attributes take no value: `autoplay` / `loop` / `click-control` / `has-play`; omit them
   when off.
5. `layout`: usually `responsive` + `width`/`height`, or `fill` / `container` depending on the
   wrapper.
6. Play button: a bare `<svg role="play">` as the play slot makes the whole card fail to render
   (the page shows an HTML comment saying the card was not found or failed to render instead of the
   card). Choose:

   | Need | How |
   |---|---|
   | Default (recommended) | No slot; write `click-control` and use the component's own controls |
   | Custom play/pause look | Put `role="play"` / `role="pause"` on a plain element (e.g. `<span>`) whose look is pure CSS or text — no `<svg>` in the slot |

   The public Nova theme uses a plain `<span role="play">` slot the same way.
7. The video field is a `video_picker` (see [schema-rules.md](../schema-rules.md)).
8. A `video_picker` value is an object, not a URL string. Shape: `video.sources[] = { type, url }`,
   plus `video.width` / `video.height` / `video.path` (`path` is the cover). Loop over the sources
   to get the addresses:

   ```liquid
   {% assign video = block.settings.video %}
   {% for source in video.sources %}
     {% if source.type == 'video/mp4' %}{% assign mp4 = source.url %}
     {% elsif source.type == 'application/x-mpegURL' %}{% assign hls = source.url %}{% endif %}
   {% endfor %}
   {% if mp4 or hls %}…{% endif %}
   ```

   Two consequences: outputting the object directly (`mp4="{{ block.settings.video }}"`) gives an
   empty string and a black player; and the empty check can't be
   `{% if block.settings.video != blank %}` — the object is never blank, so it enters the `if` even
   with no video. Always use `{% if mp4 or hls %}`. Cover fallback:
   `poster="{{ poster | default: video.path | img_url }}"`. `image_picker` doesn't have this problem;
   only `video_picker` returns an object.
9. With no source, still reserve the space: output a fixed-height placeholder; never let the media
   area collapse (see the skeleton).

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `mp4` | mp4 URL | give at least one of `mp4` / `hls` / `src` | most common; missing all → no error, just a black player |
| `hls` | hls URL | same | can be the only source |
| `poster` | Cover | yes | missing → the component errors |
| `layout` | Layout | no | usually `responsive` |
| `width` / `height` | Size | no | usually needed with `responsive` |
| `object-fit` | Fit | no | usually `cover` |
| `autoplay` / `loop` | Autoplay / loop | no | boolean, no value |
| `click-control` | Click to play/pause | no | boolean, no value |
| `has-play` | Show the custom play-button area | no | boolean, no value |
| `src` | Single URL | same as `mp4` | rarely; prefer `mp4` / `hls` |

## Actions

| Action | Effect | Parameters |
|---|---|---|
| `play` | Play | none |
| `pause` | Pause | none |
| `seekTo` | Jump to a time | `currentTime` (seconds, required; missing → the component errors) |

Rarely needed. Use only the attributes in the table; don't write `zoom` or the picture-in-picture
set.

## Events

| Event | When |
|---|---|
| `play` | Playback starts |
| `pause` | Paused (by the user, or automatically when scrolled out of view) |
| `ended` | Reached the end |
| `enterFull` | Entered fullscreen |
| `exitFull` | Left fullscreen |

## Skeleton

```liquid
{% assign video = block.settings.video %}
{% for source in video.sources %}
  {% if source.type == 'video/mp4' %}{% assign mp4 = source.url %}
  {% elsif source.type == 'application/x-mpegURL' %}{% assign hls = source.url %}{% endif %}
{% endfor %}
{% assign poster = block.settings.poster %}

{% if mp4 or hls %}
  <ljs-video
    layout="responsive"
    width="1280"
    height="720"
    {% if mp4 %}mp4="{{ mp4 }}"{% endif %}
    {% if hls %}hls="{{ hls }}"{% endif %}
    poster="{{ poster | default: video.path | img_url }}"
    object-fit="cover"
    click-control
    {% if block.settings.enable_autoplay %}autoplay{% endif %}
    {% if block.settings.enable_loop %}loop{% endif %}
  ></ljs-video>
{% elsif poster != blank %}
  <ljs-img layout="responsive" width="1280" height="720" object-fit="cover"
    src="{{ poster | img_url: '1280x' }}" alt=""></ljs-img>
{% else %}
  <div class="media-placeholder" aria-hidden="true"></div>
{% endif %}
```

Custom play/pause (rule 6, second row) — slot children inside `<ljs-video>`, drawn with CSS:

```liquid
  <ljs-video … click-control>
    <span role="play" class="{{ root_cls }}__play" aria-label="Play"></span>
    <span role="pause" class="{{ root_cls }}__pause" aria-label="Pause"></span>
  </ljs-video>

<style>
  .{{ root_cls }}__play {
    width: 0; height: 0;
    border-style: solid; border-width: 14px 0 14px 24px;
    border-color: transparent transparent transparent #fff;
  }
  .{{ root_cls }}__pause {
    width: 18px; height: 24px;
    border-left: 6px solid #fff; border-right: 6px solid #fff;
    box-sizing: border-box;
  }
</style>
```
