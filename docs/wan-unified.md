# Unified Wan 3.0

Public requests use `model: "wan3.0"` and `POST /v1/videos` with a website API key
in the official group. The four legacy names remain compatible with their
original protocols, but their model metadata may be hidden from the catalog.

| mode | speed | Required media | Upstream |
| --- | --- | --- | --- |
| general | standard (default) | optional reference lists | wan3.0-video |
| general | fast | optional reference lists | wan3.0-video-prime |
| reference | fast (default, only value) | non-empty reference lists | wan3.0-prime-r2v |
| frames | standard (default, only value) | first_frame and last_frame | wan3.0-i2v |

`mode` defaults to `auto`, selecting `frames` when either frame field is supplied,
otherwise `general`. Invalid combinations fail before quota reservation. The
server translates the same reference arrays into `media` for R2V and converts
first/last-frame URLs to I2V media. It removes its own control fields before
forwarding. Unknown fields and explicit nulls are rejected by the unified API.
The older routes retain their original validators.

Common fields are model, prompt, mode, speed, seconds/duration,
size/resolution, aspect_ratio/ratio, n, prompt_extend, first_frame, last_frame,
and reference_images/reference_videos/reference_audios. Reference items accept
url, optional image role, and optional general-mode video duration. R2V rejects
frame roles and video duration instead of silently discarding them. Image roles
are reference_image/first_frame/last_frame in general mode, and reference_image
only in reference mode. URL values must be credential-free public HTTPS URLs.
Input video duration is positive, finite and at most the common 3600-second task
safety limit; actual media compatibility remains an upstream check.

Duration is 2–30 seconds, default 5; seconds uses an integer string and duration
an integer. Resolution is 480P/720P/1080P, default 720P. Aspect ratio is
16:9/9:16/1:1, default 16:9. Aliases must agree. n may only be 1; prompt_extend is
an optional provider boolean and explicit false is preserved.

The existing 720p base price and resolution ratios also apply to wan3.0. At a
USD 0.30 base price with group ratio 1, 480p/720p/1080p cost USD 0.25/0.30/0.35
per requested second. Reservation/refund semantics are unchanged. All quantity
validation occurs before billing; tests cover every target and unsafe inputs.
The website does not add reference-video duration or resettle Wan against actual
output duration. The provider documents separate reference-duration billing, but
does not specify a complete authoritative calculation/rounding contract. Do not
infer website charges from client-supplied reference duration. Changing the
website's charging policy requires a confirmed business rule and accounting tests.

The selected upstream name is recorded before the provider checkpoint. Tasks
persist a `wan-unified` provider marker so responses keep the public model and
IDs and normalize states to queued/in_progress/completed/failed. Poll and
content endpoints use the same website key. Do not automatically retry POST
when acceptance is uncertain.

Unified Wan and new Lxmone Seedance tasks support Range/If-Range on content:
206 for partial content, 416 for an unsatisfiable range, and 200 when the provider
returns the full file (including an If-Range mismatch). Download before completion
returns 409. Legacy tasks retain their existing download behavior. The provider
advertises a configurable default 48-hour result retention; download promptly,
as the website does not guarantee permanent media storage.

## Deployment configuration

Add wan3.0 to the existing Wan channel without replacing its four old IDs. Set
its model price to the existing 720p Wan rate and use only the official group.
Create visible metadata for wan3.0; set the four old metadata entries to hidden
(status 0) to consolidate the catalog, preserving their channel abilities and
prices. Metadata visibility does not disable existing relay routes. No global
provider model mapping is necessary: the adaptor selects a target per request.

The website API tab and one-click AI guide share the unified parameter inventory
and four alternative request examples. No real API key is included in copied
content. UI translations follow the project locale sync workflow.

## Optional webhooks

`webhook_url` and `webhook_secret` are website-only fields supported by the unified
model. They are not forwarded upstream. See [video webhooks](video-webhooks.md)
for limits, payloads, signatures and retry behavior. Existing polling remains available.
