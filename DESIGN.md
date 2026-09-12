---
version: alpha
name: new-api
description: Existing gateway dashboard and developer API documentation
omitted:
  - section: colors
    reason: Runtime themes own semantic colors; this document does not duplicate theme values.
  - section: typography
    reason: Existing font tokens are owned by web/default/src/styles/theme.css.
  - section: spacing
    reason: Existing shared primitives and Tailwind utilities own spacing.
  - section: rounded
    reason: Existing radius tokens are owned by the runtime theme.
---

# new-api design context

## Product and audience

The dashboard serves gateway operators and API consumers. The pricing detail API
panel helps a developer choose valid parameters, understand charges, submit one
request and retrieve its result. It is a technical reference within the existing
product, with no new visual identity or generation controls.

Supported locales are en, zh, zh-TW, fr, ja, ru and vi. Locale availability is not
evidence of a country-specific business workflow. API field names, model IDs and
code examples retain their literal syntax; explanations and controls use i18next.

## Canonical ownership

- Semantic colors, light/dark modes, type and radii: `web/default/src/styles/theme.css`
  and `theme-presets.css`, consumed by the Tailwind theme adapter and shared UI.
- Parameters: `StaticDataTable`; preserve readable wrapping of long field names.
- Code and individual copy actions: `CodeBlock` and `CodeBlockCopyButton`.
- Whole-guide copy: shared `Button`, `copyToClipboard`, and Sonner success/error
  feedback, following sibling model API documentation panels.
- Webhook documentation: shared `VideoWebhookDocs`.
- Wan code examples and copied guide: `features/pricing/lib/wan-api-docs.ts`.
  The Markdown reference is generated from the same guide function.

This file records established ownership. It does not generate or override runtime
tokens. No page-local color, font, dialog, table or copy implementation is needed.

## Documentation behavior

Use existing section spacing, muted explanatory text, semantic headings and
underlined source links. Keep optional parameters distinct from required input.
Examples must match backend validation and specify inexpensive test settings.
Display defaults, reserve versus final charges, provider restrictions and recovery
steps explicitly. Copy actions provide success or failure feedback. Source links
open with `noopener noreferrer`. Long technical text must wrap without widening
the page. Preserve keyboard operation and the existing focus-visible styles.

Do not add decorative animation, new font families or alternate component styles
to routine API reference updates. Verify the rendered table, examples and copied
text against the same released contract.
