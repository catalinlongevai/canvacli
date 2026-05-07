# Pages, Resize, Brand Kit APIs (v2.0 research)

Source: [llms.txt](https://www.canva.dev/docs/connect/llms.txt) and [api.yml](https://www.canva.dev/sources/connect/api/latest/api.yml).
All endpoints verified live against `DAF7Q_-7g9Q` ("Project DW", 13-page presentation, 1920x1080) using the user's stored token on 2026-05-07.

Base URL for all calls: `https://api.canva.com/rest`.

## Pages

**Endpoint:** `GET /v1/designs/{designId}/pages`
**Scope:** `design:content:read`
**Rate limit:** 100 / client-user
**Status:** Preview API (Canva warns of unannounced breaking changes; preview APIs disqualify a public integration from review).

Query params:
- `offset` (1-based, default 1, max 500)
- `limit` (default 50, max 200)

Response shape (verified):
```json
{
  "items": [
    {
      "index": 1,
      "dimensions": { "width": 1920.0, "height": 1080.0 },
      "thumbnail": {
        "width": 595, "height": 335,
        "url": "https://document-export.canva.com/.../thumbnail/0001.png?X-Amz-..."
      }
    }
  ]
}
```

Notes:
- `index` is the only required field; `dimensions` and `thumbnail` are present for paged design types but absent for unbounded ones (whiteboards) per the `PageDimensions` schema doc ("if it is bounded").
- Some design types have no pages at all (Canva docs).
- Pagination uses `offset`/`limit` (not the `continuation` token used elsewhere). For 13-page Project DW we got 5 items with `limit=5`; need a second call with `offset=6` for the rest. The response has no `continuation` or `next_offset` field, so the client must compute it.
- Total page count comes from `GET /v1/designs/{id}` -> `design.page_count` (13 for Project DW). `/pages` itself does not return a total.
- Thumbnail URLs are signed S3 URLs valid for several hours.

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://api.canva.com/rest/v1/designs/DAF7Q_-7g9Q/pages?offset=1&limit=5"
```

## Per-page export

**There is no separate per-page endpoint.** Export uses the regular async export pipeline:

**Endpoint:** `POST /v1/exports` -> async; poll `GET /v1/exports/{exportId}`.
**Scope:** `design:content:read`
**Rate limit:** 20 / client-user, plus integration throttle (750 / 5 min, 5,000 / 24 h), document throttle (75 / 5 min), user throttle (75 / 5 min, 500 / 24 h).

The selector lives inside `format`. Every multi-page-capable format has a `pages: integer[]` field (1-based page numbers; omit to export all):

| Format | `pages` array? | Notes |
|---|---|---|
| `png` | yes | One image per requested page; one download URL per page |
| `jpg` | yes | Same |
| `pdf` | yes | Single PDF containing only requested pages |
| `gif` | yes | Animated GIF over selected pages |
| `pptx` | (no `pages`) | Whole-deck only |
| `mp4` | (no `pages`) | Whole-deck only |
| `html_bundle`, `html_standalone` | (no `pages`) | Whole-design only |

Verified PNG of page 2 only:
```bash
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"design_id":"DAF7Q_-7g9Q","format":{"type":"png","pages":[2]}}' \
  "https://api.canva.com/rest/v1/exports"
# -> {"job":{"id":"1b4666be-...","status":"in_progress"}}
# Poll GET /v1/exports/{id} until status="success", then job.urls[] holds the signed PNG.
```

Implementation note for `canva pages export <design-id> <page-num> --format png`: build `format.pages = [page-num]` and reuse the existing exports polling code path. For `--format pdf` of one page, same thing -> the API returns a 1-page PDF.

## Resize

**Endpoint:** `POST /v1/resizes` -> async; poll `GET /v1/resizes/{jobId}`.
**Scopes:** `design:content:read` AND `design:content:write` (both required).
**Rate limit:** 20 (POST) / 120 (GET) per client-user.
**Capability:** `x-required-capabilities: resize`. The user must be on a Canva plan with premium features (Pro or above), or Canva grants a small free trial — see "Implementation notes" below.

Side effects (verified): Always creates a NEW design; original is untouched (`updated_at` of `DAF7Q_-7g9Q` did not change after resize). The new design lands in the user's root projects folder. In-place resize is not exposed in Connect.

Other restrictions from the spec: max 25,000,000 px squared; multi-page resizes affect every page; Canva docs / emails / Canva Code designs cannot be resized to or from.

Request body — `oneOf` two shapes via discriminator `type`:

Preset (verified `whiteboard` works):
```json
{ "design_id": "DAF7Q_-7g9Q", "design_type": { "type": "preset", "name": "presentation" } }
```
The full enum of `PresetDesignTypeName` is exactly these four values:
- `doc`
- `email`
- `presentation`
- `whiteboard`

There is no `instagram_post`, `instagram_story`, `a4_document`, `facebook_cover`, etc. in the public Connect API. Anything beyond those four MUST go through the custom branch:

Custom:
```json
{ "design_id": "DAF7Q_-7g9Q",
  "design_type": { "type": "custom", "width": 1080, "height": 1920 } }
```
Width/height each 40-8000 px, both required.

Response (verified):
```json
{ "job": { "id": "8ba518ec-...", "status": "success",
  "result": { "design": { "id": "DAHJAuJHXD8", "title": "Project DW",
                          "page_count": 13, "urls": {...}, "thumbnail": {...} },
              "trial_information": { "uses_remaining": 1, "upgrade_url": "..." } } } }
```
Statuses: `in_progress`, `success`, `failed`. Error codes: `thumbnail_generation_error`, `design_resize_error`, `create_design_error`, `trial_quota_exceeded`.

Implementation note for `canva resize <design-id> --to <preset>`: maintain a tiny client-side map. Treat `presentation`, `doc`, `email`, `whiteboard` as preset-mode; map common aliases (`instagram_story` -> custom 1080x1920, `instagram_post` -> 1080x1080, `a4_document` -> 2480x3508, `facebook_cover` -> 1640x624, `youtube_thumbnail` -> 1280x720, `twitter_post` -> 1600x900, etc.) to custom mode. Document this in `--help` so users know the difference.

## Brand Kit

**There is no public Brand Kit API.** Verified: `GET /v1/brand-kits`, `/v1/brand-kit`, `/v1/brands`, `/v1/brand`, `/v1/brand-kits/me` all return HTTP 404. The OpenAPI spec has zero matches for `brand_kit`, `brand-kit`, or `BrandKit`.

The only "brand" surface in Connect is **Brand Templates**, which is a different concept — they are starting templates with autofill placeholder fields, not the brand color/font/logo library.
- `GET /v1/brand-templates` — list (scope `brandtemplate:meta:read`)
- `GET /v1/brand-templates/{id}` — get one (scope `brandtemplate:meta:read`)
- `GET /v1/brand-templates/{id}/dataset` — autofill schema (scope `brandtemplate:content:read`)

Brand Templates require Canva Enterprise; for the test user (free tier), `GET /v1/brand-templates` returns `{"items": []}` with HTTP 200, not 403.

There is no documented Connect endpoint that returns brand colors, fonts, or logos as structured data.

## Rate limits

| Endpoint | Limit |
|---|---|
| `GET /v1/designs/{id}/pages` | 100 / client-user |
| `POST /v1/exports` | 20 / client-user, plus 750/5min and 5000/24h per integration, 75/5min per document, 75/5min and 500/24h per user |
| `GET /v1/exports/{id}` | (poll; same `export` tag) |
| `POST /v1/resizes` | 20 / client-user |
| `GET /v1/resizes/{id}` | 120 / client-user |
| `GET /v1/brand-templates` | (Enterprise only; not relevant to free tier) |

## Implementation notes

- **Pages is a preview API** — pin a CHANGELOG entry, and surface a `--quiet` flag so the warning doesn't spam scripts. Don't ship `canva pages` as a flagship command without noting upstream instability.
- **Brand kit on free accounts: not supported, period.** Don't ship `canva brand-kit`. If it must exist, scope it down to a stub that prints "Canva Connect does not expose brand kit data; only brand templates (Enterprise) are available" and link `/docs/connect/api-reference/brand-templates/`. Or wire it to `/v1/brand-templates` and rename the command to `canva brand-templates` — that endpoint at least exists.
- **Resize and free tier:** the test user (free) successfully ran one resize and got back `trial_information.uses_remaining: 1`. Canva grants a small trial quota even outside Pro; after that, expect `trial_quota_exceeded`. CLI should surface `trial_information.upgrade_url` and `uses_remaining` to the user when present.
- **Resize preset list is tiny.** Confirm with `canva resize --list-presets` that only the four official preset names are documented as preset; everything else must be custom dimensions. Otherwise users will assume `canva resize design --to instagram_story` calls a Canva-defined preset when in fact it is a client-side alias.
- **Per-page export reuses the exports pipeline.** No new polling code needed — extend the existing `format` builder to accept `pages: []int`. PNG/JPG return one URL per page in `job.urls[]`; PDF returns one URL containing the slice.
- **Pages list has no totals or continuation token.** Use `design.page_count` from `GET /v1/designs/{id}` if you need a total, and increment `offset` by `limit` until `len(items) < limit`.
- **Thumbnail URLs expire** (signed S3, ~hours). Don't cache them; re-fetch the page list when needed.

## What's NOT supported

- **No Brand Kit API** in Canva Connect (verified 404 on every plausible path; no OpenAPI matches for brand kit colors/fonts/logos). Do not invent the command.
- **No per-page export endpoint distinct from `/v1/exports`.** Per-page is a `format.pages` array on the regular export job.
- **No Canva-defined size presets beyond `doc`/`email`/`presentation`/`whiteboard`** for resize. "Instagram story", "A4 document", etc. only exist as client-side aliases mapped to `custom` width/height.
- **No in-place resize** — resize always forks a new design.
- **No `pages` array for `pptx` / `mp4` / `html_bundle` / `html_standalone` exports.** These are whole-design only.

Status: DONE
