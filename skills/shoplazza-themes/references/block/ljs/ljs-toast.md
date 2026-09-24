# ljs-toast — toast message

Short feedback message or form-validation hint, shown with `showToast`.

## Rules

1. Use `ljs-toast`. No hand-written fixed overlay + `setTimeout` show/hide.
2. Add `hidden`; give it a stable `id`.
3. Trigger: `@tap="{{ toast_id }}.showToast(content='...')"`. Set the duration with the `duration`
   attribute or the action's `duration=` parameter.
4. Boolean attributes take no value. `show` is the component's display state; don't make it a schema
   setting.
5. Form validation: `validation-for` + `visible-when-invalid` (value attributes, not booleans).

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `duration` | Display time in ms | no, default `6000` | can also be passed to `showToast` |
| `validation-for` | Id of the input it validates | no | validation case |
| `visible-when-invalid` | Validation state that shows it | no | e.g. `valueMissing`, `patternMismatch` |
| `target` | CSS selector of a parent to move the toast into | no | instead of the default position |

Actions: only `showToast(content=, duration=)` — `content=` is required, `duration=` optional
(overrides the attribute). No custom events. Use only the attributes in the table; names like
`type` or `placement` are not recognised. The child slot `<span role="content">` (where the message
renders) is rarely needed.

## Skeleton

```liquid
{% capture toast_id %}toast-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <button type="button" @tap="{{ toast_id }}.showToast(content='{{ block.settings.toast_message | escape }}')">
    {{ block.settings.button_text | default: 'Show toast' }}
  </button>

  <ljs-toast
    id="{{ toast_id }}"
    layout="nodisplay"
    hidden
    {% if block.settings.duration != blank %}duration="{{ block.settings.duration }}"{% endif %}
  ></ljs-toast>
</div>
```
