# Lxmone Seedance integration

The `lxmone.xyz` OpenAI channel uses its own Seedance protocol. Argolink channels
keep their existing validation, `/v1/videos/generations` submission, resolution
pricing, reference-video multiplier, reserve, and response format.

| Public model | Upstream model | Duration | Effective resolutions |
| --- | --- | --- | --- |
| seedance-2 | seedance-2-pro | 4–15 seconds | 480p, 720p, 1080p, 4k |
| seedance-2-fast | seedance-2-fast | 5 or 10 seconds | 480p, 720p; 1080p becomes 720p |
| seedance-2-mini | seedance-2-mini | 5 or 10 seconds | 480p, 720p; 1080p becomes 720p |
| seedance-2.5 | seedance-2.5-pro | 4–30 seconds | 480p, 720p, 1080p; 4k becomes 1080p |

## Requests and polling

Use `POST /v1/videos`, JSON, and a website API key in the official group. Defaults
are 5 seconds, 720p, and 16:9. Both integer and string `seconds` are accepted;
`duration` is an integer alias. Conflicting aliases are rejected before billing.
The normalized duration and resolution are used for both billing and the upstream
request. `stream`, `n`, `response_format`, and `webhook_url` are unsupported.

First-frame `input_reference` / `image` and last-frame `image_end` /
`end_image_url` pass through unchanged. A last frame requires a first frame;
frame mode cannot be combined with `reference_*`. Reference audio on 2.0 models
requires reference images or videos. Upstream validation handles media URLs and
provider-specific media limits.

Save the website's returned `id`; query `GET /v1/videos/{id}` and download
`GET /v1/videos/{id}/content` with the same website key. Tasks persist their
provider identity, so polling/response conversion and settlement do not change
when another channel using the same public model is re-enabled.

## Isolated channel pricing

Do not change the shared `seedance-2.5` model price to configure Lxmone: Argolink
can still use it. Set `other_settings.lxmone_seedance_resolution_ratios` on the
Lxmone channel. It is a nested map keyed by **upstream model name**, then
**effective lowercase resolution**. Each value is:

`desired per-second base price / configured public ModelPrice`

For example, a public ModelPrice of 0.50 and a desired 480p price of 0.20 require
a 480p channel ratio of 0.40. These numbers illustrate the formula; they are not
provider prices. The existing user-group ratio still applies.

All requested tiers require an explicit finite positive ratio. Missing prices
fail with `model_price_error` before upstream submission or quota reservation;
the adaptor never falls back to Argolink's hardcoded ratios. Each public model
also needs a positive ModelPrice. Prices are not supplied by the public provider
documentation and must be confirmed before enabling the channel.

Duration and the selected channel ratio are saved with the billing snapshot.
When the provider returns delivered duration, settlement uses it with the saved
ratio and saturated quota math. Without delivered duration, the original charge
remains. Standard failed-task refunds are unchanged.

The model catalog displays the highest-priority enabled provider's tiers and
protocol. With equal priorities, the lower channel ID wins catalog selection;
use different priorities when providers have different price contracts. Both
cached and database routing exclude providers that cannot serve the request path.
Channel 145's original model prices and request contract remain available when
it is re-enabled. Channel-local tier ratios must be configured before testing.

Provider contract: https://lxmone.xyz/user-api-docs.html?base_url=https%3A%2F%2Flxmone.xyz%2Fv1#seedance
