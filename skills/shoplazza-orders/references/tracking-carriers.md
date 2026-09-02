# orders tracking-carriers — carrier lookup

Read-only lookup of the tracking carriers Shoplazza Fulfillment supports, plus detection of
which carrier owns a tracking number.

**Boundary:** registering an external real-time rate-quote carrier is `shop carrier-services`
(`shoplazza-shop`). Lookup / detection lives HERE.

## Commands

| Intent | Command |
|---|---|
| Which carrier is tracking number X? (运单号识别) | `orders tracking-carriers detect --params '{"tracking_number":"<no>"}'` → `.data.tracking_carrier` |
| List all supported carriers | `orders tracking-carriers list` → `.data.tracking_carriers[]` |

- `detect` takes exactly one required query param, `tracking_number`. Use it directly — do
  not `list` and match by hand.
- Both are reads: answer from `.data`, nothing fabricated, no `--dry-run` theater.
- Carrier names/codes from `list` are also what `+ship --company` / `--company-code` accept.
