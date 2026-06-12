# Checkout Extension

A Shoplazza checkout extension project.

## Authenticate (global, keychain-based)

    shoplazza auth login --store-domain <your-store>.myshoplaza.com

## Develop

    shoplazza checkout dev --extension-id <extension-name>   # or --all

Opens a local dev server on http://localhost:8888 with live rebuild.

## Build & publish

    shoplazza checkout build --id <extension-name>
    shoplazza checkout push  --extension-id <extension-name>

> This project intentionally has no `extension.config.js` — the store and token
> come from the global `shoplazza auth` context, not a file in the repo.
