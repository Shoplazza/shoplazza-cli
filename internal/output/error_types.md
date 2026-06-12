# Error type mapping reference

v2 `internal/output` defines exactly 5 error envelope types. All shortcut /
internal packages MUST classify their error paths into one of these — no
new types may be introduced.

| `Type*`        | exit code | meaning                                            |
|----------------|-----------|----------------------------------------------------|
| `TypeAPI`      | 1         | server returned a 4xx/5xx or task ended in failure |
| `TypeValidation` | 2       | local input or local guard rejected                |
| `TypeAuth`     | 3         | unauthenticated, 401/403, OAuth refresh failed     |
| `TypeNetwork`  | 4         | timeout / DNS / dial / TLS / live-server bind     |
| `TypeInternal` | 5         | OS resource / panic / unexpected crash             |

## v1 → v2 mapping (theme module, source of truth)

| v1 error source                                          | v2 type      | code | notes                                       |
|----------------------------------------------------------|--------------|------|---------------------------------------------|
| not authenticated / keychain miss / OAuth refresh fail   | auth         | 3    | hint runs `shoplazza auth login`            |
| HTTP 401 / 403                                           | auth         | 3    | token-class regardless of message           |
| flag/arg validation (`--theme-id` missing, format error) | validation   | 2    | local; immediate                            |
| HTTP 400 / 422 server-side business validation           | validation   | 2    | service rejected user input                 |
| local-path validation (ParseThemeFile, .themeignore)     | validation   | 2    | local guard                                 |
| Pack/Unpack rejection (traversal, >100/200MB, corrupt)   | validation   | 2    | local guard                                 |
| HTTP 404 / resource not found / task not found           | api          | 1    | server normally answered                    |
| HTTP 5xx                                                 | api          | 1    | message passthrough                         |
| task ended status=2 (server-side failure)                | api          | 1    | server answered, business failed            |
| task polling timeout (3min)                              | network      | 4    | special envelope shape; see Decision 16     |
| DNS / dial / TLS handshake / read timeout / reset        | network      | 4    | real network layer                          |
| livereload port bind failed (in-use / permissions)       | network      | 4    | Decision 14                                 |
| fsnotify watcher fatal (EMFILE / loss / perms revoked)   | internal     | 5    | OS resource; hint to change OS config       |
| local file I/O (read theme dir, write tmp, disk full)    | internal     | 5    | local non-validation                        |
| panic / unknown unwrap / recover()                       | internal     | 5    | last-resort                                 |
| v1 `process.exit(-1)` legacy sites                       | internal     | 5    | catchall                                    |

## Adding new rows

When a new module migrates and finds an error class missing from the table,
append a row here and reference it from the module's `errors.go`. PR review
gates: any new envelope `type` that isn't in the table above (e.g., `remote`)
SHALL be rejected.
