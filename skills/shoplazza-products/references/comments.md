# products comments — reference

Customer **reviews** on products (买家评价/评论), including bulk review import. This is
catalog data — not storefront content (blogs / pages → `shoplazza-shop`).

Subcommands: `create` · `batch-create` · `list`. There is no get/update/delete.
Verify shapes with `shoplazza schema products.comments.<cmd>`.

| Command | HTTP | Params / body |
|---|---|---|
| `comments create` | `POST /comments` | `--data '{"comment":{…}}'` |
| `comments batch-create` | `POST /comments/batch` | `--data '{"comments":[{…},…]}'` — bulk import (批量导入评价) |
| `comments list` | `GET /comments/list` | filters: `product_id`, `status`, `sort_by`, date ranges, cursor paging |

## Required fields — more than you'd expect

`comment` requires ALL of: `product_id` · `user_name` · `star` (integer rating) · `like`
(integer like-count) · `created_at` · `content`. Optional: `country`, `images`.

For an import where the source data lacks a field, use honest neutral values (`like: 0`), and
ask the user for anything that shapes the review itself (star rating, content) — don't invent
opinions on their behalf.

```bash
# Import two reviews for one product
products comments batch-create --data '{
  "comments": [
    {"product_id":"555777","user_name":"Alice","star":5,"like":0,
     "created_at":"2026-06-01T10:00:00Z","content":"Great quality!"},
    {"product_id":"555777","user_name":"Bob","star":4,"like":0,
     "created_at":"2026-06-03T18:30:00Z","content":"Fits well."}
  ]}' --dry-run          # batch write: user go-ahead first, then run without --dry-run

# List a product's reviews
products comments list --params '{"product_id":"555777"}'
```
