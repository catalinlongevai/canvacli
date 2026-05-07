# Assets + Imports APIs (v2.0 research)

Status: **DONE**

Researched against Canva Connect OpenAPI `2024-06-18`
([api.yml](https://www.canva.dev/sources/connect/api/latest/api.yml)) and live-tested
where the user's token had scope. Base URL: `https://api.canva.com/rest`.

Doc index: <https://www.canva.dev/docs/connect/llms.txt>

---

## Asset upload

**Two flavors**, both async-job-based:

| Endpoint | Body | Purpose |
| --- | --- | --- |
| `POST /v1/asset-uploads` | `application/octet-stream` raw bytes | Upload a local file |
| `POST /v1/url-asset-uploads` | `application/json` `{name, url}` | Have Canva fetch a public URL (preview API) |

The bytes endpoint is the one canvacli should ship; URL-based is still flagged as
preview and capped at 100 MB for video.

**Request shape — bytes upload.** The filename + tags don't go in the body; they go
in an `Asset-Upload-Metadata` HTTP header that is itself a JSON object whose
`name_base64` field is the Base64-encoded asset name (max 50 chars unencoded). Allows
emoji and Unicode through HTTP headers.

```bash
NAME_B64=$(printf 'My Awesome Upload' | base64)
curl -X POST https://api.canva.com/rest/v1/asset-uploads \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/octet-stream" \
  -H "Asset-Upload-Metadata: {\"name_base64\":\"$NAME_B64\"}" \
  --data-binary @photo.png
```

**Response — async job.** Returns `200` immediately with `{job:{id,status}}` where
`status ∈ {in_progress, success, failed}`. Poll
`GET /v1/asset-uploads/{jobId}` until terminal.

On `success`, the response embeds the full `asset` object:

```json
{"job":{
  "id":"e08861ae-3b29-45db-8dc1-1fe0bf7f1cc8",
  "status":"success",
  "asset":{
    "id":"Msd59349ff",
    "type":"image",
    "name":"My Awesome Upload",
    "tags":["image","holiday"],
    "owner":{"user_id":"oU123...","team_id":"oB123..."},
    "created_at":1377396000,"updated_at":1692928800,
    "thumbnail":{"width":595,"height":335,"url":"https://document-export.canva.com/..."}
  }
}}
```

On `failed`, `error.code` is one of `file_too_big | import_failed | fetch_failed`.

**Asset ID format.** `M` prefix + 8 hex chars (e.g. `Msd59349ff`). This is the same ID
shape that goes into `DatasetImageValue.asset_id` for autofill — see "Cross-cutting"
below.

**File limits & formats** (from [Assets overview](https://www.canva.dev/docs/connect/api-reference/assets/)):

- **Images, max 50 MB:** JPEG, PNG, HEIC, single-frame GIF, TIFF, single-frame WEBP.
- **Videos, max 500 MB (bytes upload), 100 MB (URL upload):** M4V, MKV, MP4, MPEG,
  QuickTime, WebM.

**Required scope:** `asset:write` (POST), `asset:read` (GET job).

**Rate limit:** `POST /v1/asset-uploads` 30 req/client-user; `GET .../{jobId}` 180 req/client-user.

---

## Asset list

**There is no list-assets endpoint.** `GET /v1/assets` returns
`404 endpoint_not_found`. Verified against the live API:

```
$ curl -H "Authorization: Bearer $TOKEN" https://api.canva.com/rest/v1/assets
HTTP/1.1 404
{"code":"endpoint_not_found","message":"Unknown endpoint GET /v1/assets"}
```

**Closest substitute — folder browse:** `GET /v1/folders/{folderId}/items` returns
items of `type ∈ {design, folder, image}` (note: **video assets are explicitly
excluded** per the OpenAPI description). Pagination via opaque `continuation` token
+ `limit` (1–100, default 50). Filters: `item_types` (csv), `sort_by`, `pin_status`.
Use `folderId=root` for the user's top-level folder.

**Implication for canvacli `assets list`:** ship it as
`canva folders items <folderId> --type image` (rebrand) or document it as
"image-only, folder-scoped". Don't promise a flat asset library listing —
the Connect API doesn't expose one.

**Required scope:** `folder:read`. **Rate limit:** 100 req/client-user.

---

## Asset get

`GET /v1/assets/{assetId}` returns the same `Asset` object embedded in the upload
job result, plus type-specific `metadata` (image: width/height/smart_tags; video:
width/height/duration). `import_status` is also available for assets that came from
imports.

**URL expiry.** The `thumbnail.url` is a presigned S3 URL
(`https://document-export.canva.com/...?X-Amz-Algorithm=...`). Live-observed
import response had `X-Amz-Expires=65561` (~18 hours) and a `response-expires`
header about ~18 h out. **Treat thumbnail URLs as short-lived; re-fetch the asset
to get a fresh URL rather than caching the URL itself.** The OpenAPI does not state
an expiry guarantee — code defensively.

`PATCH /v1/assets/{assetId}` updates `name` (≤50 chars) and/or `tags`
(≤50 items × 50 chars). Tags **replace**, not merge.
`DELETE /v1/assets/{assetId}` returns 204; mirrors UI trash (does not remove from
designs that already use the asset).

**Required scopes:** `asset:read` (GET), `asset:write` (PATCH/DELETE).
**Rate limits:** GET 100, PATCH 30, DELETE 30 req/client-user.

---

## Imports — file → Canva design

**Endpoints:**

| Endpoint | Body | Purpose |
| --- | --- | --- |
| `POST /v1/imports` | `application/octet-stream` raw bytes | Local file → design |
| `POST /v1/url-imports` | JSON `{title, url, mime_type?}` | URL → design |
| `GET /v1/imports/{jobId}` | — | Poll bytes-import job |
| `GET /v1/url-imports/{jobId}` | — | Poll URL-import job |

**Request shape** (bytes). Title goes in an `Import-Metadata` header
(`{"title_base64":"...", "mime_type":"application/pdf"}`); `mime_type` is optional
(Canva sniffs if omitted).

**Live-tested.** Imported a hand-crafted 1-page PDF and a 3-page PDF using the
user's token:

```bash
TITLE_B64=$(printf 'My doc' | base64)
curl -X POST https://api.canva.com/rest/v1/imports \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/octet-stream" \
  -H "Import-Metadata: {\"title_base64\":\"$TITLE_B64\",\"mime_type\":\"application/pdf\"}" \
  --data-binary @doc.pdf
# → {"job":{"id":"c3a74e34-...","status":"in_progress"}}

curl -H "Authorization: Bearer $TOKEN" \
  https://api.canva.com/rest/v1/imports/c3a74e34-...
# → status:"success", result.designs:[{id:"DAHJAjJSyvE", page_count:1, ...}]
```

**Multi-page handling.** Confirmed live: a 3-page PDF imports as a **single Canva
design** with `page_count: 3` (not three separate designs). The OpenAPI description
on `DesignImportJobResult.designs` says: _"A list of designs imported from the
external file. It usually contains one item. Imports with a large number of pages
or assets are split into multiple designs."_ — so always iterate `result.designs[]`
defensively, but for typical 5–20 slide decks expect a single design.

The response shape includes `page_count` per design (live-observed, not in the
OpenAPI examples), plus `id`, `title`, `thumbnail`, `urls.{edit_url,view_url}`,
`created_at`, `updated_at`. The `urls.edit_url`/`view_url` JWTs include
`expiry` claims (live-observed ~7 days out); thumbnail URLs are presigned S3
~18 hours.

**Supported file types** (per [Design imports overview](https://www.canva.dev/docs/connect/api-reference/design-imports/)):

- **PDF** (`application/pdf`) — works.
- **Microsoft Office** — `.pptx`, `.docx`, `.xlsx` and legacy `.ppt`/`.doc`/`.xls`.
- **Apple iWork** — Keynote, Pages, Numbers.
- **Adobe** — Illustrator (`.ai`), Photoshop (`.psd`).
- **Affinity** — `.af`, `.afdesign`, `.afphoto`, `.afpub`.
- **OpenOffice** — Draw, Impress, Calc, Writer.

**PNG / JPEG are NOT supported by `/v1/imports`.** They go through
`/v1/asset-uploads` instead — imports are for documents, asset uploads for media.
This is an important UX decision for canvacli's `import` command: detect the
extension and route image files to asset-upload.

**Async pattern.** Same `{job:{id,status}}` shape as asset uploads. Live observation:
1-page PDF completed within first 2 s poll; 3-page PDF likewise. Use the standard
exponential backoff per
[API requests and responses](https://www.canva.dev/docs/connect/api-requests-responses/)
("start with a short poll interval, then exponentially increase ... up to a maximum
interval"). Recommend: 500 ms → 1 s → 2 s → 4 s → 8 s, cap 8 s, total deadline 60 s
for documents.

`error.code` enum: `design_creation_throttled | design_import_throttled |
duplicate_import | internal_error | invalid_file | fetch_failed`.

**Required scope:** `design:content:write` (both POST and GET).
**Rate limits:** POST 20, GET 120 req/client-user.

---

## Cross-cutting: how assets connect to autofill

This is the load-bearing question for the agent-driven deck workflow. **It works
exactly as hoped.**

The autofill request body uses a `data: {<field_name>: DatasetValue, ...}` map.
For image fields the `DatasetValue` discriminator is `type: "image"` and the
payload is just `{type:"image", asset_id:"..."}` — verbatim from the OpenAPI:

```yaml
DatasetImageValue:
  properties:
    type: { enum: [image] }
    asset_id:
      description: '`asset_id` of the image to insert into the template element.'
      example: Msd59349ff
  required: [asset_id, type]
```

**The `asset_id` here is the same `Msd59349ff`-format ID returned by
`POST /v1/asset-uploads` → poll → `job.asset.id`.** No transformation, no separate
"design library" step. The full v2.0 flow:

1. `POST /v1/asset-uploads` with image bytes → `job.id`.
2. Poll `GET /v1/asset-uploads/{job.id}` → `job.asset.id` (e.g. `Msd59349ff`).
3. `POST /v1/autofills` with
   `data: {hero_image: {type:"image", asset_id:"Msd59349ff"}, headline: {type:"text", text:"..."}}`.
4. Poll `GET /v1/autofills/{job.id}` → `job.result.design.id` + `urls.edit_url`.

**Concrete example from OpenAPI** (line 4139 of `api.yml`, abbreviated):

```json
{
  "brand_template_id": "DAFVztcvd9z",
  "data": {
    "cute_pet_image_of_the_day": {"type":"image","asset_id":"Msd59349ff"},
    "cute_pet_witty_pet_says":   {"type":"text","text":"It was like this when I got here!"}
  }
}
```

This means canvacli's agent can: (a) generate or fetch images, (b) upload them as
assets, (c) hand the resulting `M…` IDs to the autofill data map alongside text
and chart fields. No intermediate design-library import is required.

---

## Required scopes (summary)

| Operation | Scope |
| --- | --- |
| Create / poll asset upload (bytes or URL) | `asset:write` (POST) / `asset:read` (GET) |
| Get / patch / delete asset | `asset:read` / `asset:write` / `asset:write` |
| List folder items (the only "list assets" path) | `folder:read` |
| Create / poll design import (bytes or URL) | `design:content:write` |
| Create / poll autofill | `design:content:write` (and Enterprise membership) |

The current canvacli token has `design:content:write` (live-confirmed via successful
import) but **lacks `asset:read` and `asset:write`** (live: 403 `missing_scope`).
v2.0 must request these in OAuth — recommended scope set:
`asset:read asset:write design:content:read design:content:write design:meta:read folder:read`.

---

## Rate limits (per-client-user, summary)

| Endpoint | Limit |
| --- | --- |
| `POST /v1/asset-uploads`, `POST /v1/url-asset-uploads` | 30 |
| `GET /v1/asset-uploads/{jobId}`, `GET /v1/url-asset-uploads/{jobId}` | 180 |
| `GET /v1/assets/{id}` | 100 |
| `PATCH /v1/assets/{id}`, `DELETE /v1/assets/{id}` | 30 |
| `POST /v1/imports`, `POST /v1/url-imports` | 20 |
| `GET /v1/imports/{jobId}`, `GET /v1/url-imports/{jobId}` | 120 |
| `GET /v1/folders/{id}/items` | 100 |

Window units aren't documented explicitly in the OpenAPI extension
`x-rate-limit-per-client-user`; the Canva docs describe them as per-minute
sliding windows in the rate-limits page. Treat poll endpoints (180/120) as roomy,
create endpoints (20/30) as tight — back off aggressively on 429.

---

## Implementation notes for canvacli v2.0

- **CLI surface.** `canva assets upload <file> [--name N] [--tag T...]`,
  `canva assets get <id>`, `canva assets rm <id>`, `canva assets tag <id> ...`.
  **Drop `canva assets list`** (or document it as
  `canva folders items <id> --type image` — there is no flat list endpoint).
- **`canva import <file>`.** Detect MIME by extension; if image → route to
  `assets upload` and print a hint that imports are for documents. PPTX/PDF/Keynote
  → `/v1/imports` with `Import-Metadata`. The result's `page_count` and `urls.edit_url`
  are the user-facing fields to print.
- **Polling.** Reuse one helper for asset-uploads / imports / autofills / exports —
  identical `{job:{id,status,result?,error?}}` envelope and exponential backoff.
  Recommend: 500 ms → cap 8 s, deadline 60 s for imports/uploads, 120 s for autofills.
- **Asset-ID flow into autofill.** `assets upload` should print the bare `M...` ID
  to stdout (so `canva autofill --image-field hero=$(canva assets upload hero.png)`
  composes cleanly in shell). Already idiomatic with v1's design-id printing.
- **Thumbnail URL caching.** Don't. They're presigned S3 with ~18 h expiry —
  re-`GET /v1/assets/{id}` when the user asks again.
- **Header sizing.** `Asset-Upload-Metadata` and `Import-Metadata` are JSON inside
  HTTP headers; with `name_base64` ≤ ~70 bytes encoded, no header-size concerns.
- **Multipart.** Both upload endpoints take **raw octet-stream**, NOT multipart.
  Don't reach for `multipart/form-data` libraries.
- **No PNG-as-import.** Live-tested: imports rejects unsupported formats with
  `400 unsupported or un-recognised format`. Code paths must distinguish "render to
  design" (imports) from "add to media library" (asset uploads).
