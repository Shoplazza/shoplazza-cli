# L0 drift self-test fixture (NOT a real skill)

Planted drift for verifying `lint_drift.mjs` — especially the `unknown-body-field`
body-key check (Task A). Run:

```
node skills/shoplazza-skill-eval/bin/lint_drift.mjs \
  skills/shoplazza-skill-eval/bin/drift_fixture.md \
  --svc none --backbone skip --bin ./shoplazza
```

Expected: **exactly one** finding — `unknown-body-field: billing one-time create --data .nmae`.
Every other block below must stay silent (proving the false-positive guards). If any
control block starts flagging, a guard regressed.

## MUST FLAG — planted typo (`nmae` should be `name`)

```bash
billing one-time create --data '{"application_charge":{"nmae":"Setup Fee","price":"9.99","return_url":"https://app.example.com/done"}}'
```

## MUST NOT flag — freeform value subtree (metafields `value` is caller-shaped)

`value` nests a recursive google.protobuf.Value; keys under it are arbitrary.

```bash
shop metafields-resource create --data '{"definition_id":"D1","namespace":"specs","key":"material","type":"single_line_text_field","value":{"mediaContentType":"IMAGE","image":{"path":"x.jpg","alt":""}}}'
```

## MUST NOT flag — freeform map (analytics `filters` is keyed by dimension)

```bash
shop analytics overview --data '{"begin_time":"1","end_time":"2","indicator":["sales"],"filters":{"country_code":"US","made_up_dim":"x"}}'
```

## MUST NOT flag — placeholder value skips the whole check

An ellipsis (or `<id>`, `$VAR`) means the example is illustrative; skip it.

```bash
billing one-time create --data '{"application_charge":{"name":"…","totally_made_up":"x"}}'
```

## MUST NOT flag — negated gotcha cell documents a wrong field on purpose

| Symptom | Fix |
|---|---|
| `webhook create --data '{"webhook":{"bogus_field":"x"}}'` fails | use `topic` + `address` |
