# Canva Connect API — Research Notes for `canvacli`

Source of truth (in order of precedence):
- OpenAPI spec: <https://www.canva.dev/sources/connect/api/latest/api.yml>
- Reference docs: <https://www.canva.dev/docs/connect/>
- Index for crawlers: <https://www.canva.dev/docs/connect/llms.txt>

All endpoints documented below were cross-checked against the OpenAPI spec on 2026-05-07.

## Global facts

- **Base URL:** `https://api.canva.com/rest/v1` (note the `/rest/v1` prefix, not just `/v1`).
- **Auth header:** `Authorization: Bearer {access_token}` for every Connect endpoint except the OAuth token endpoint, which uses HTTP Basic.
- **Content type:** `application/json` for JSON bodies, `application/x-www-form-urlencoded` for the OAuth token endpoint.
- **Token TTL:** access tokens last `expires_in: 14400` seconds (4 hours). Refresh tokens are returned alongside and rotate on use.
- **Rate-limit headers:** Canva's public docs do **not** document `X-RateLimit-*` headers. We must rely on `429 too_many_requests` responses and parse `Retry-After` if present, with exponential backoff otherwise. (See <https://www.canva.dev/docs/connect/api-requests-responses/>.)
- **Idempotency keys:** No `Idempotency-Key` header is documented in the spec. Treat POSTs as non-idempotent; for retries on POST (autofill, export, create design) we should de-dupe client-side using a hash of (template_id + data) or by polling for in-flight jobs after a 5xx.
- **Error shape (verbatim):**
  ```json
  { "code": "design_not_found", "message": "Design with id 'ABCD' not found" }
  ```
  Mapping to internal codes lives in the implementation plan; full code list at <https://www.canva.dev/docs/connect/error-responses.md>.

### Go-friendly base types

```go
type APIError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

type Thumbnail struct {
    Width  int    `json:"width"`
    Height int    `json:"height"`
    URL    string `json:"url"` // 15-min expiry
}

type DesignURLs struct {
    EditURL string `json:"edit_url"` // 30-day expiry
    ViewURL string `json:"view_url"` // 30-day expiry
}

type Owner struct {
    UserID string `json:"user_id"`
    TeamID string `json:"team_id"`
}
```

---

## 1. `canva login` — OAuth 2.0 PKCE

Docs: <https://www.canva.dev/docs/connect/authentication/>, <https://www.canva.dev/docs/connect/api-reference/authentication/generate-access-token.md>

### Authorization endpoint (browser redirect)

`GET https://www.canva.com/api/oauth/authorize`

Query parameters:

| Param | Required | Notes |
|---|---|---|
| `response_type` | yes | Must be `code` |
| `client_id` | yes | The integration's client ID |
| `scope` | yes | **Space-separated** scope strings (URL-encoded as `%20`). Scopes are explicit — `asset:write` does **not** imply `asset:read` |
| `code_challenge` | yes | Base64URL(SHA-256(code_verifier)) |
| `code_challenge_method` | yes | Must be `S256` |
| `redirect_uri` | recommended | Must match a URI registered for the client |
| `state` | recommended | High-entropy random string for CSRF |

Verbatim example:
```
https://www.canva.com/api/oauth/authorize?code_challenge=eeeAbcdefgh123456789Vz96F9UIv8EHwnmibz3Djx3EE&code_challenge_method=s256&scope=asset:read%20asset:write%20design:meta:read%20folder:read%20comment:write&response_type=code&client_id=OCABC12-DeF
```

### Token endpoint (code exchange & refresh)

`POST https://api.canva.com/rest/v1/oauth/token`
- `Content-Type: application/x-www-form-urlencoded`
- `Authorization: Basic base64(client_id:client_secret)` (or `client_id`/`client_secret` in the body for public clients)

```sh
curl --request POST 'https://api.canva.com/rest/v1/oauth/token' \
  --header 'Authorization: Basic {credentials}' \
  --header 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=authorization_code' \
  --data-urlencode 'code_verifier=...' \
  --data-urlencode 'code=...' \
  --data-urlencode 'redirect_uri=https://example.com/process-auth'
```

Refresh: same endpoint with `grant_type=refresh_token` and `refresh_token=...`.

Response:
```json
{
  "access_token": "...",
  "refresh_token": "...",
  "token_type": "Bearer",
  "expires_in": 14400,
  "scope": "design:meta:read design:content:read ..."
}
```

### Verified scope identifiers (from <https://www.canva.dev/docs/connect/appendix/scopes.md>)

`asset:read`, `asset:write`, `brandtemplate:meta:read`, `brandtemplate:content:read`, `comment:read`, `comment:write`, `design:meta:read`, `design:content:read`, `design:content:write`, `folder:read`, `folder:write`, `folder:permission:write`, `profile:read`, `collaboration:event`, `openid`, `profile`, `email`.

**Minimum scope set for `canvacli` v1:** `design:meta:read design:content:read design:content:write brandtemplate:meta:read brandtemplate:content:read folder:read profile:read`. (Add `asset:write` later if/when we support uploading inputs for autofill image fields.)

Companion endpoints we should ship as `canva login --status` / `canva logout`:
- Introspect: `POST /rest/v1/oauth/introspect`
- Revoke: `POST /rest/v1/oauth/revoke`

---

## 2. `canva templates` — list brand templates

Docs: <https://www.canva.dev/docs/connect/api-reference/brand-templates/list-brand-templates.md>

`GET /rest/v1/brand-templates`

| Query | Default | Notes |
|---|---|---|
| `query` | — | Free-text search |
| `limit` | 25 | 1–100 |
| `continuation` | — | Cursor returned by previous page |
| `ownership` | `any` | `any` \| `owned` \| `shared` |
| `sort_by` | `relevance` | `modified_descending` etc. |
| `dataset` | `any` | `non_empty` to filter to autofill-capable templates |

Scope: `brandtemplate:meta:read`. Rate limit: 100 rpm/user. **Requires Canva Enterprise** (true for every brand-template endpoint).

```bash
curl 'https://api.canva.com/rest/v1/brand-templates?limit=50' \
  -H 'Authorization: Bearer {token}'
```

```json
{
  "items": [
    {
      "id": "DAFVztcvd9z",
      "title": "Q3 Sales Deck",
      "view_url": "https://www.canva.com/...",
      "create_url": "https://www.canva.com/...",
      "created_at": 1377396000,
      "updated_at": 1692928800,
      "thumbnail": { "width": 595, "height": 335, "url": "https://..." }
    }
  ],
  "continuation": "RkFGMgXlsVTDbMd:..."
}
```

```go
type BrandTemplate struct {
    ID        string    `json:"id"`
    Title     string    `json:"title"`
    ViewURL   string    `json:"view_url"`
    CreateURL string    `json:"create_url"`
    CreatedAt int64     `json:"created_at"`
    UpdatedAt int64     `json:"updated_at"`
    Thumbnail Thumbnail `json:"thumbnail"`
}

type ListBrandTemplatesResponse struct {
    Items        []BrandTemplate `json:"items"`
    Continuation string          `json:"continuation,omitempty"`
}
```

Pagination: present `continuation` if more pages exist; absent/empty when done. Loop until missing.

> **Heads-up:** Brand template IDs were rotated in September 2025; old IDs are accepted for 6 months from rotation. Ship a soft warning if a saved ID looks like the legacy format.

---

## 3. `canva templates show <id>` — metadata + autofill dataset

Two endpoints; the CLI should call both and merge.

### Metadata
Docs: <https://www.canva.dev/docs/connect/api-reference/brand-templates/get-brand-template.md>
`GET /rest/v1/brand-templates/{brandTemplateId}` — scope `brandtemplate:meta:read`, 100 rpm/user.

```json
{ "brand_template": { "id": "...", "title": "...", "view_url": "...", "create_url": "...", "created_at": 1377396000, "updated_at": 1692928800, "thumbnail": {...} } }
```

### Dataset (autofill fields)
Docs: <https://www.canva.dev/docs/connect/api-reference/brand-templates/get-brand-template-dataset.md>
`GET /rest/v1/brand-templates/{brandTemplateId}/dataset` — scope `brandtemplate:content:read`, 100 rpm/user.

```json
{
  "dataset": {
    "cute_pet_image_of_the_day": { "type": "image" },
    "cute_pet_witty_pet_says":   { "type": "text" },
    "cute_pet_sales_chart":      { "type": "chart" }
  }
}
```

```go
type DatasetField struct {
    Type string `json:"type"` // "image" | "text" | "chart"
}

type GetBrandTemplateDatasetResponse struct {
    Dataset map[string]DatasetField `json:"dataset"`
}
```

> Chart fields are flagged "preview" — gate behind a `--include-preview` flag and emit a warning on use.

---

## 4. `canva create --template --autofill` — async autofill job

Docs: <https://www.canva.dev/docs/connect/api-reference/autofills/create-design-autofill-job.md>, <https://www.canva.dev/docs/connect/api-reference/autofills/get-design-autofill-job.md>

### Create

`POST /rest/v1/autofills` — scope `design:content:write`, **60 rpm/user**, Enterprise-only.

```json
{
  "brand_template_id": "DAFVztcvd9z",
  "title": "Weekly newsletter — May 2026",
  "data": {
    "headline":     { "type": "text", "text": "Hello world" },
    "hero_image":   { "type": "image", "asset_id": "Msd59349ff" },
    "sales_chart":  { "type": "chart", "chart_data": { "column_configs": [...], "rows": [...] } }
  }
}
```

Response (immediate, with `in_progress` likely):
```json
{ "job": { "id": "AbC123", "status": "in_progress" } }
```

### Poll

`GET /rest/v1/autofills/{jobId}` — scope `design:meta:read`, 60 rpm/user.

Status union: `in_progress` | `success` | `failed`.

Success:
```json
{
  "job": {
    "id": "AbC123",
    "status": "success",
    "result": {
      "type": "create_design",
      "design": {
        "id": "DAFxyz",
        "title": "Weekly newsletter — May 2026",
        "url": "https://www.canva.com/design/...",
        "urls": { "edit_url": "...", "view_url": "..." },
        "thumbnail": { "width": 595, "height": 335, "url": "..." },
        "created_at": 1714857600,
        "updated_at": 1714857600,
        "page_count": 4
      }
    }
  }
}
```

Failure: `job.error.code` is one of `autofill_error`, `thumbnail_generation_error`, `create_design_error`, `design_approval_error`.

**Polling pattern:** start at 1s, exponential backoff capped at 5s, max wall time ~120s by default (override via `--timeout`). Stop on terminal status. Same pattern as the export job poller — share the implementation.

---

## 5. `canva list` — list designs

Docs: <https://www.canva.dev/docs/connect/api-reference/designs/list-designs.md>

`GET /rest/v1/designs` — scope `design:meta:read`, 100 rpm/user.

| Query | Default | Notes |
|---|---|---|
| `query` | — | Max 255 chars |
| `continuation` | — | Cursor |
| `ownership` | `any` | `any` \| `owned` \| `shared` |
| `sort_by` | `relevance` | `modified_descending`, `modified_ascending`, `title_descending`, `title_ascending` |
| `limit` | 25 | 1–100 |

Response per item:
```json
{
  "id": "DAFVztcvd9z",
  "title": "My summer holiday",
  "owner": { "user_id": "...", "team_id": "..." },
  "urls": { "edit_url": "...", "view_url": "..." },
  "created_at": 1377396000,
  "updated_at": 1692928800,
  "thumbnail": { "width": 595, "height": 335, "url": "..." },
  "page_count": 5
}
```

```go
type Design struct {
    ID        string     `json:"id"`
    Title     string     `json:"title"`
    Owner     Owner      `json:"owner"`
    URLs      DesignURLs `json:"urls"`
    CreatedAt int64      `json:"created_at"`
    UpdatedAt int64      `json:"updated_at"`
    Thumbnail Thumbnail  `json:"thumbnail"`
    PageCount int        `json:"page_count"`
}
type ListDesignsResponse struct {
    Items        []Design `json:"items"`
    Continuation string   `json:"continuation,omitempty"`
}
```

Pagination contract is identical to brand templates: missing/empty `continuation` ⇒ end of list.

---

## 6. `canva export <id> --format pdf` — async export

Docs: <https://www.canva.dev/docs/connect/api-reference/exports/create-design-export-job.md>, <https://www.canva.dev/docs/connect/api-reference/exports/get-design-export-job.md>

### Create

`POST /rest/v1/exports` — scope `design:content:read`. Multiple rate limits stack:
- **20 rpm/user** (general)
- **75 exports / 5min** per design and per user
- **500 exports / 24h** per user
- Integration-wide: 750 / 5min, 5000 / 24h

```json
{
  "design_id": "DAVZr1z5464",
  "format": { "type": "pdf", "size": "a4", "pages": [2, 3, 4] }
}
```

Supported `format.type` values and key options:

| Type | Options |
|---|---|
| `pdf` | `export_quality` (`regular`/`pro`), `size` (`a4`/`a3`/`letter`/`legal`), `pages` |
| `jpg` | `quality` 1–100 (req'd), `export_quality`, `width`/`height` 40–25000, `pages` |
| `png` | `export_quality`, `width`/`height`, `lossless`, `transparent_background`, `as_single_image`, `pages` |
| `gif` | `export_quality`, `width`/`height`, `pages` |
| `mp4` | `quality` (e.g. `horizontal_1080p`), `export_quality`, `pages` |
| `pptx` | `pages` |
| `html_bundle`, `html_standalone` | `pages` (single page only) |

Response:
```json
{ "job": { "id": "ExpAbC", "status": "in_progress" } }
```

### Poll

`GET /rest/v1/exports/{exportId}` — scope `design:content:read`, **120 rpm/user**.

```json
{
  "job": {
    "id": "ExpAbC",
    "status": "success",
    "urls": ["https://document-export.canva.com/..."]
  }
}
```

`urls` is **valid for 24 hours** — download immediately on success. Failure codes: `license_required`, `approval_required`, `internal_failure`. Reuse the same polling primitive as autofill.

For `--format pdf` specifically, default to `{ "type": "pdf", "export_quality": "regular" }` and let `--paper a4|letter` toggle `format.size`.

---

## 7. `canva delete <id>` — **NOT SUPPORTED IN PUBLIC API**

Verified absent from the OpenAPI spec on 2026-05-07. There is no `DELETE /rest/v1/designs/{designId}`. The spec exposes DELETEs for assets and folders only:
- `DELETE /rest/v1/assets/{assetId}` — soft-trashes an asset
- `DELETE /rest/v1/folders/{folderId}` — soft-trashes a folder (returns 204; user's content goes to Trash, others' content moves to their projects root)

**Implications for the CLI:**
- Either omit `canva delete` from v1, or implement it as a "move to a `.canvacli/trash` folder" workaround using `POST /rest/v1/folders/move` (path: `/rest/v1/folders/move`, scope `folder:write`).
- Document loudly in `--help` that this is a folder-move, not a real delete, and that real deletion requires the Canva web UI.
- File a tracking issue to revisit when Canva ships the endpoint.

---

## 8. `canva folders` — list folders / list folder items

Docs: <https://www.canva.dev/docs/connect/api-reference/folders/list-folder-items.md>, <https://www.canva.dev/docs/connect/api-reference/folders/get-folder.md>, <https://www.canva.dev/docs/connect/api-reference/folders/create-folder.md>

The Connect API has **no top-level "list all folders" endpoint**. Folders are traversed by walking from the root. Confirmed in the spec — only these folder paths exist: `POST /folders`, `GET|PATCH|DELETE /folders/{folderId}`, `GET /folders/{folderId}/items`, `POST /folders/move`.

### Special folder IDs (from the create-folder reference)
- `"root"` — top of the user's projects.
- `"uploads"` — the Uploads folder.

So `canva folders` should default to `GET /rest/v1/folders/root/items?item_types=folder`.

`GET /rest/v1/folders/{folderId}/items` — scope `folder:read`, 100 rpm/user.

| Query | Default | Notes |
|---|---|---|
| `continuation` | — | Cursor |
| `item_types` | all | Comma-delimited subset of `design`, `folder`, `image` |
| `limit` | 50 | Max 100 |
| `sort_by` | — | e.g. `created_ascending`, `title_descending` |
| `pin_status` | `any` | `pinned` \| `any` |

Item shape is a tagged union:
```json
{ "items": [
  { "type": "folder", "folder": { "id": "...", "name": "...", "created_at": 0, "updated_at": 0, "thumbnail": {...} } },
  { "type": "design", "design": { "id": "...", "title": "...", "url": "...", "urls": {...}, "page_count": 3, "thumbnail": {...} } },
  { "type": "image",  "image":  { "type": "image", "id": "...", "name": "...", "tags": [], "thumbnail": {...} } }
] , "continuation": "..." }
```

Note: video assets are **not** returned. Don't promise full asset listing in the v1 CLI.

```go
type FolderItem struct {
    Type   string         `json:"type"` // discriminator
    Folder *FolderSummary `json:"folder,omitempty"`
    Design *Design        `json:"design,omitempty"`
    Image  *ImageAsset    `json:"image,omitempty"`
}
```

---

## 9. `canva whoami` — current user

Two endpoints, called in parallel.

`GET /rest/v1/users/me` — **no scope required**, 10 rpm/user.

```json
{ "team_user": { "user_id": "auDAbliZ2rQNNOsUl5OLu", "team_id": "Oi2RJILTrKk0KRhRUZozX" } }
```

`GET /rest/v1/users/me/profile` — scope `profile:read`, 10 rpm/user.

```json
{ "profile": { "display_name": "Jane Doe" } }
```

Combine into a single CLI table:
```go
type WhoAmI struct {
    UserID      string `json:"user_id"`
    TeamID      string `json:"team_id"`
    DisplayName string `json:"display_name"`
}
```

The 10 rpm/user limit is the tightest in the API — cache the result per-token in the local config for 5 minutes.

---

## Cross-cutting design notes for implementation

1. **Polling primitive.** Three async jobs (autofill, export, asset upload, design import) use the same `{ job: { id, status, result?, error? } }` envelope with `in_progress|success|failed`. Build one generic `pollJob[T any]` helper with backoff + cancellation context.
2. **Pagination primitive.** `items + continuation` cursor is reused across `/designs`, `/brand-templates`, `/folders/{id}/items`. Single iterator helper.
3. **Error mapping.** Map Canva `code` → stable internal codes. Cluster: `*_not_found` → `ENotFound`, `bad_request_*` / `invalid_*` / `missing_*` → `EInvalidArgument`, `invalid_access_token` / `revoked_access_token` → `EUnauthenticated`, `missing_scope` / `unauthorized_*` → `EPermissionDenied`, `too_many_requests` / `quota_exceeded` → `EResourceExhausted`, `internal_error` → `EInternal`. Always preserve raw `code` in the error chain for `--debug`.
4. **No idempotency keys.** For POST retries on transport-layer failure, do not blindly resend — instead, on `canva create` either surface the failure or implement client-side dedupe via a hash stored alongside the saved-job state file.
5. **No documented rate-limit headers.** Honor `Retry-After` if servers send it; otherwise back off `429s` with jitter (250ms → 8s, max 5 retries). Surface `quota_exceeded` distinctly from `too_many_requests` since the former is daily and not retry-able in-session.
6. **URL expiries.** Thumbnails: 15 min. `edit_url`/`view_url`: 30 days. Export `urls`: 24h. The CLI must download exports immediately on success rather than printing a link the user might click later.
7. **Enterprise-gating.** Brand templates and autofill require Canva Enterprise. Detect upfront with a single `GET /brand-templates?limit=1` probe and surface a friendly message if it 403s with `unauthorized_user` / `missing_scope`.
