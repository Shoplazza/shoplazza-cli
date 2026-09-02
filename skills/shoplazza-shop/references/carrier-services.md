# shop carrier-services — real-time shipping-rate carriers (实时运费报价)

A carrier service registers an **external endpoint that quotes real-time shipping rates
at checkout** — the platform calls the merchant's `callback_url` with the cart/address
and displays the returned rates. This is checkout rate *quoting*, NOT fulfillment
tracking (that's `orders tracking-carriers`) and NOT zone/rate tables
(`orders shipping-schemas`).

## Commands

```bash
shop carrier-services list
shop carrier-services get    --params '{"carrier_service_id":"<id>"}'
shop carrier-services create --data '{"carrier_service":{"name":"…","callback_url":"https://…","carrier_code":"…"}}'
shop carrier-services update --params '{"carrier_service_id":"<id>"}' --data '{"carrier_service":{"callback_url":"https://…","active":true}}'
shop carrier-services delete --params '{"carrier_service_id":"<id>"}'   # --dry-run first
```

## Required fields

- `create` (`carrier_service` wrapper object): `name`, `callback_url`, `carrier_code` —
  all **required**. `callback_url` is the user's own rate-quoting endpoint — **never
  invent one**; ask if missing. Optional: `active` (bool), `logo`, `short_desc`.
- `update`: the schema requires **both** `callback_url` and `active` in the body — even
  to flip only `active`, resend the current `callback_url` (read it first via `get`).

## Boundaries

| Sounds like | Actually |
|---|---|
| 实时运费报价 / checkout rate quoting | **HERE** — `shop carrier-services` |
| 物流商 / 运单跟踪 / tracking providers | `orders tracking-carriers` (orders domain) |
| 运费方案 / shipping zones & rate tables | `orders shipping-schemas` (orders domain) |
