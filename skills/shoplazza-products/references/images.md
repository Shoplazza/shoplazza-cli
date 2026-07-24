# products images — reference

Images **attached to a product** (gallery / main image). This is not the store media
library — uploading a standalone asset file is `shop files` / `shop +upload-file`
(`shoplazza-shop` skill). Rule of thumb: "给商品 X 加图" → here; "上传文件/图片到店铺(媒体库)"
→ shop.

All commands are product-scoped: `product_id` is a path param via `--params`.
Verify shapes with `shoplazza schema products.images.<cmd>`.

| Command | HTTP | Params / body |
|---|---|---|
| `images list` | `GET /products/{product_id}/images` | `--params '{"product_id":"<id>"}'` |
| `images count` | `GET /products/{product_id}/images/count` | same |
| `images get` | `GET /products/{product_id}/images/{image_id}` | `--params '{"product_id":"…","image_id":"…"}'` |
| `images create` | `POST /products/{product_id}/images` | `--data '{"image":{"src":"<url>"}}'` — `src` is the only required body field |
| `images update` | `PUT /products/{product_id}/images/{image_id}` | body `image.{src,alt,position,…}` |
| `images delete` | `DELETE /products/{product_id}/images/{image_id}` | destructive → `--dry-run` first |

Optional body fields on create/update: `alt`, `position` (gallery order), `width`, `height`.
Set them only when the user asks — never fabricate.

```bash
# Attach an image by URL
products images create --params '{"product_id":"889900"}' \
  --data '{"image":{"src":"https://cdn.example.com/new-photo.jpg"}}'

# Make an existing image the first in the gallery
products images update --params '{"product_id":"889900","image_id":"<img>"}' \
  --data '{"image":{"position":1}}'
```

The image must already be reachable at a public URL — the CLI does not upload local files
here. For a local file, first put it somewhere public (e.g. the store media library via
`shop +upload-file`, which also takes public URLs, not local paths), then pass the resulting
URL as `image.src`.

A **variant-specific** image is set through the variant, not here: `variants update` with
`variant.image_id` or `variant.image.src` (see [variants.md](variants.md)).
