# ljs-script — custom runtime logic

Custom runtime logic, for when the intent → component table in [selection.md](selection.md) has
no component for the job. Per the public
[spz-script docs](https://lessjs.shoplazza.com/latest/components/spz-script/), the script runs
asynchronously in a Web Worker and may be at most 15KB.

It is not for static JSON config — for that use `<script type="application/json">` read with
`src="script:<id>"`. When a component in [selection.md](selection.md) covers the need, don't
reimplement it here (a second implementation fights the component for the DOM).

## Rules

1. Don't write a `type` attribute.
2. `scope` is required: its value is the id of the element the script may work on (usually the
   block root).
3. No bare `<script>` in the theme that reads or writes the DOM; custom logic always goes through
   this component.
4. Don't send requests to other origins. Don't use `innerHTML`, `outerHTML`, or `eval`-style
   dynamic code; write text with `textContent`, or render markup with an
   [ljs-render](ljs-render.md) template.
5. Read every changeable value (copy, durations, counts…) from `block.settings.*`; hard-coded values
   can't be changed by the merchant in the editor.
6. It must have a stable `id`; `layout` is `logic` ([writing.md](writing.md)).
7. Export functions with `exportFunction('name', fn)`; call them with
   `<id>.execute(func='name', params=...)`.
8. To feed a renderer: `src="spz-script:<id>.<function>"`. The prefix is `spz-script:` even though
   the tag is `ljs-script` — plugin components rename only the tag (`spz-` → `ljs-`); other usages
   stay unchanged ([plugin docs](https://lessjs.shoplazza.com/latest/docs/plugin/)). Use the
   function name you actually exported.
9. Write the script body inline inside the tag. Use only the attributes in the table.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `id` | Component id | yes | stable; `execute` and data feeding rely on it |
| `layout` | Layout | yes | `logic` |
| `scope` | Element the script works on | yes | id of the target element (value attribute, not a boolean) |

Actions: only `execute` (`func=` / `params=`; `params` may be an object, array, or string). There
are no custom events, but `@<functionName>` fires when that exported function succeeds, and
`@<functionName>Error` when it fails (e.g. `@setLabelError="..."`), for error handling.

## Skeleton (exported functions + execute + feeding a render)

```liquid
{% capture script_id %}script-{{ block_id }}{% endcapture %}
{% capture result_id %}result-{{ block_id }}{% endcapture %}

<div id="{{ root_cls }}" class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-script layout="logic" id="{{ script_id }}" scope="{{ root_cls }}">
    const state = {
      prefix: {{ block.settings.prefix | default: 'Recommended: ' | json }},
      label: {{ block.settings.default_label | json }}
    };

    function setLabel(event) {
      var raw = event && event.value != null ? event.value : '';
      state.label = state.prefix + raw;
      return Promise.resolve({});
    }

    function getData() {
      return Promise.resolve(state);
    }

    exportFunction('setLabel', setLabel);
    exportFunction('getData', getData);
  </ljs-script>

  <input
    type="text"
    name="label"
    placeholder="{{ block.settings.placeholder | escape }}"
    @input-debounced="{{ script_id }}.execute(func='setLabel', params=event);{{ result_id }}.render"
  >

  <ljs-render id="{{ result_id }}" layout="container" src="spz-script:{{ script_id }}.getData">
    <template>
      <p class="script-result">${data.label}</p>
    </template>
  </ljs-render>
</div>
```
