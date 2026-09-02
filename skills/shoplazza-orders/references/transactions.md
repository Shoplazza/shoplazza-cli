# orders transactions — buyer payment transactions

Payment events attached to ONE order: status, payment channel, amount, gateway response.
Read-only — a single endpoint.

**Boundary:** these are the *buyer's* payments on an order (订单支付流水). App charges the
platform bills the *merchant* are `billing transactions` (`shoplazza-billing`) — do not mix
them up.

## Command

```bash
orders transactions list --params '{"order_id":"<id>"}'          # → .data.transactions[]
orders transactions list --params '{"order_id":"<id>","status":"success"}'
orders transactions list --params '{"order_id":"<id>","payment_channel":"paypal"}'
```

Optional query filters (only pass them when the user asks — don't invent filters):

| Param | Values |
|---|---|
| `status` | `authorized` · `void` (cancelled) · `processing` · `success` · `error` · `refunding` · `refunded` · `refund_failed` · `expire` |
| `payment_channel` | channel name, e.g. `paypal` |
