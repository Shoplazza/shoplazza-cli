# ljs-currency — amounts in the store's currency

Use it for every amount the card shows in the store's currency: product prices (current /
compare-at / price range / discount amount) and amounts the merchant types in (a pricing table, for
instance). The component renders `value` with the store's currency symbol, symbol position and
money format — never assemble the symbol yourself. lessjs reference:
[spz-currency](https://lessjs.shoplazza.com/latest/components/spz-currency/) (write the tag as
`ljs-currency`).

## Rules

1. The amount goes in the `value` attribute, never as child text. `value` takes a number: pass
   product fields directly (`{{ p.price }}`), without `money_with_symbol`. Convert merchant-typed
   amounts to the current market's currency with `| market_rate_exchange` first (product prices
   are already market prices; don't convert them).
2. `layout`: see the single-value table in [writing.md](writing.md) R4.
3. To put several amounts on one price line, give `ljs-currency` `display: inline-block` in the
   card's CSS; wrap a compare-at price's component in `<s>`.
4. Fixed marketing copy that never changes currency (e.g. a "¥99 today only" button label) can be
   plain text; use the component only when the amount must follow the store's currency / format.
5. Never invent attribute names.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `value` | Amount | Yes | The only place the amount goes |
| `format` | Number format | No | Defaults to the store's money format. Values: `amount`, `amount_no_decimals`, `amount_with_comma_separator`, `amount_no_decimals_with_comma_separator`, `amount_with_apostrophe_separator` |
| `symbol` | Currency symbol | No | Defaults to the store's |
| `symbol-position` | `left` / `right` | No | Defaults to the store's |
| `container-class` | Extra class on the inner `.money` element | No | For coloring |
| `code` | Currency code (`USD`, `EUR`…) instead of `symbol` + `symbol-position` | No | Rarely needed |

## Examples

Product price line (current + compare-at), inside a product loop where `p` is the product and
`price_now` / `show_compare` were set earlier:

```liquid
<div class="{{ root_cls }}__price">
  <ljs-currency class="{{ root_cls }}__price-now" layout="container" value="{{ price_now }}"></ljs-currency>
  {% if show_compare %}
    <s class="{{ root_cls }}__price-was"><ljs-currency layout="container" value="{{ p.compare_at_price }}"></ljs-currency></s>
  {% endif %}
</div>
```

```css
.{{ root_cls }}__price ljs-currency { display: inline-block; }
.{{ root_cls }}__price-was { opacity: .5; }
```

Merchant-typed amount:

```liquid
<ljs-currency
  layout="container"
  value="{{ block.settings.price | market_rate_exchange }}"
></ljs-currency>
```

Custom format (rarely needed):

```liquid
<ljs-currency
  layout="container"
  value="{{ block.settings.price | market_rate_exchange }}"
  format="amount_with_comma_separator"
  container-class="price-text"
></ljs-currency>
```
