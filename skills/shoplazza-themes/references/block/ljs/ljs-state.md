# ljs-state — persistent toggle

Remembers what the buyer clicked. One click and `ljs-state` writes `true`/`false` to browser
storage; after a reload — even after leaving and coming back — the toggle is as the buyer left it.
Front-end only: no backend, not tied to an account (gone in another browser or a private window).

How long it remembers is set by `state-type`:

| Lifetime | How | Cleared when | Typical use |
|---|---|---|---|
| Long-term (default) | omit, or `state-type="localStorage"` | the buyer clears browser data | favourites, view preference, an announcement closed for good |
| This session | `state-type="sessionStorage"` | the tab is closed; shown again next visit | an offer tip already shown this visit, a one-time guide overlay |

The test is "should it be remembered next time?": yes → default; only avoid repeating within this
visit → `sessionStorage`.

Typical cases: product favourite (tap the heart → still red after reload), an announcement bar that
stays closed, an "I understand" checkbox, a grid/list view switch that remembers the preference.

When to use it:

| Need | Use |
|---|---|
| Remember a click after reload | `ljs-state` |
| Another node reads the toggle to change style / copy | `ljs-state` + `ljs-render` |
| Open/close that only matters during this page view | host `@tap` / `ljs-accordion` / `ljs-tabs`, not this |
| Store numbers, arrays, objects | not possible — `toggle` only flips a boolean |
| Sync across devices / accounts | needs a backend; this can't do it |

## Storage model

`ljs-state` is one JSON dictionary stored at `localStorage[state-key]` (or
`sessionStorage[state-key]`):

```js
localStorage["demo_fav_key"] = '{"liked": true, "closed": false}'
//           dictionary key = state-id (field name), value = boolean
```

- `state-key`: the storage slot (the storage key the whole dictionary lives under).
- `state-id`: a field name in the dictionary. One `ljs-state` can manage several fields;
  `state-id` is only the default one.
- `toggle` flips `data[state_id]`, writes it back to storage, and fires `stateChange`.
- `getState` returns `{ [state_id]: boolean }` — in the template use `${data.liked}`, not
  `${data.liked.state}`.
- Never clicked → the field isn't in storage → the value is `undefined` (falsy); render it as "off".

## Rules

1. Use `ljs-state`. No hand-written `localStorage` or global variables faking state that survives
   reload.
2. `layout`: take the value from the single-value table in [writing.md](writing.md).
3. `state-key` is required (without it nothing is saved and a reload loses the state). Always write
   `state-id` too, otherwise `getState` returns the whole dictionary.
4. `state-type` has exactly two valid values: `localStorage` (default, may be omitted) and
   `sessionStorage`. Don't abbreviate (`local` / `session` / `true`).
5. Toggle through the action: `@tap="{{ state_id }}.toggle()"`; for a non-default field use
   `toggle(state_id='closed')`.
6. Read the state only through [ljs-render](ljs-render.md) with
   `src="custom:{{ state_id }}.getState"` (read that doc too), and re-render with
   `@stateChange="{{ render_id }}.render"`.
7. `stateChange` doesn't bubble: bind it on `<ljs-state>` itself; a parent never receives it.
8. The first Liquid render doesn't know the local state, so the button's selected state must come
   from the `ljs-render` template. Don't guess `class="active"` in Liquid.
9. Derive `id` from `block_id`; append `item.id` for instances in a loop.

## What it can do

| Capability | How | Check |
|---|---|---|
| Persistent toggle | `state-key` + `state-id` + `toggle` | state survives reload |
| Session toggle | add `state-type="sessionStorage"` | reset after closing the tab |
| Several fields in one component | `toggle(state_id='closed')` | fields don't affect each other |
| Read one field | `src="custom:id.getState"` | template gets `{state_id: bool}` |
| Read the whole dictionary | `getState` without `state-id` | all fields |
| Update UI on change | `@stateChange="render_id.render"` | UI updates right after the click |

## Attributes

| Attribute | Purpose | Required | Valid values |
|---|---|---|---|
| `state-key` | Storage slot | yes | any stable string |
| `state-id` | Default field name | always write it | e.g. `liked` / `closed` |
| `state-type` | Storage | no | `localStorage` (default) / `sessionStorage` |

Actions: `toggle(state_id?)`.
Method for `custom:` reads: `getState(state_id?)`.
Events: `stateChange` (payload `{<field>: <new value>}`, doesn't bubble).

Plain collapse and tab switching don't need it (that's [ljs-accordion](ljs-accordion.md) /
[ljs-tabs](ljs-tabs.md)); and it can't store counts, arrays, or objects — `toggle` only flips a
boolean.

## Skeleton (favourite button, survives reload)

```liquid
{% capture state_id %}state-{{ block_id }}{% endcapture %}
{% capture render_id %}state-render-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-state
    id="{{ state_id }}"
    layout="container"
    state-key="fav-{{ block_id }}"
    state-id="liked"
    @stateChange="{{ render_id }}.render"
  >
    <ljs-render
      id="{{ render_id }}"
      layout="container"
      src="custom:{{ state_id }}.getState"
    >
      <template>
        <button
          type="button"
          class="fav-btn"
          data-liked="${data.liked ? 'true' : 'false'}"
          @tap="{{ state_id }}.toggle()"
        >
          ${data.liked ? '{{ block.settings.on_label }}' : '{{ block.settings.off_label }}'}
        </button>
      </template>
    </ljs-render>
  </ljs-state>
</div>
```

Style the selected state with `[data-liked="true"]`; don't build class strings in the template.

### Session "close the tip bar" (second field)

```liquid
<ljs-state
  id="{{ state_id }}"
  layout="container"
  state-key="tips-{{ block_id }}"
  state-id="closed"
  state-type="sessionStorage"
  @stateChange="{{ render_id }}.render"
>
  <ljs-render id="{{ render_id }}" layout="container" src="custom:{{ state_id }}.getState">
    <template>
      <div spz-if="${!data.closed}" class="tips-bar">
        <span>{{ block.settings.tips }}</span>
        <button type="button" @tap="{{ state_id }}.toggle(state_id='closed')">×</button>
      </div>
    </template>
  </ljs-render>
</ljs-state>
```

Both skeletons rely on `ljs-render`; read [ljs-render.md](ljs-render.md) before writing. This is the
only way to show the saved state on first paint: Liquid can't read browser storage while rendering,
so a static button can't reflect it.
