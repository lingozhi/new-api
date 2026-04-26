# Playful Geometric — shared primitives

Canonical design-system components for the Playful Geometric migration. Every
page touched in Phases 3–9 of the roadmap must consume these primitives
rather than re-gluing border/shadow/transition recipes inline.

> **Rule**: if you're tempted to write `!border-2 !border-playful-foreground shadow-pop …`
> on a new surface, use `<StickerCard>` instead. If you want a chunky pill
> button, use `<StickerButton>`. No ad-hoc recipes.

## Importing

```jsx
import {
  StickerCard,
  StickerButton,
  PlayfulKicker,
  SquiggleDivider,
  DotGridBackdrop,
  ConfettiShapes,
  ConfettiShape,
  FloatingIconBadge,
  SectionHeader,
  PlayfulTable,
  PlayfulFormField,
  PlayfulModal,
  PlayfulTag,
  PlayfulEmpty,
  PlayfulToastContainer,
} from '../common/ui/playful';
```

A dev-only gallery route mounts at `/playful-gallery` (only in `bun run dev` /
Vite dev builds) and renders every primitive in every meaningful variant. Use
it as the reviewer's one-stop visual audit.

## Design tokens

All primitives consume the tokens declared in `src/index.css:20–54` and
`tailwind.config.js`. You **should not** redefine colors, shadows, or fonts
locally.

| Concept | Token |
|---|---|
| Foreground text / border stroke | `--playful-foreground` / `text-playful-foreground` / `border-playful-foreground` |
| Background | `--playful-bg` / `bg-playful-bg` |
| Card surface | `--playful-card` / `bg-playful-card` |
| Muted text | `--playful-muted-fg` / `text-playful-muted-fg` |
| Accents (rotate for "confetti" rhythm) | `playful-accent` (violet), `playful-secondary` (pink), `playful-tertiary` (yellow), `playful-quaternary` (mint) |
| Hard shadow | `shadow-pop`, `shadow-pop-sm`, `shadow-pop-hover`, `shadow-pop-pink`, `shadow-pop-mint`, `shadow-pop-tertiary`, `shadow-pop-violet` |
| Bouncy transition | `transition-playful` utility (Tailwind plugin) |
| Fonts | `font-outfit` (display), `font-jakarta` (body) |

## Components

### StickerCard

The canonical container. Replaces every inline sticker recipe.

| Prop | Type | Default | Notes |
|---|---|---|---|
| `tone` | `'neutral' \| 'violet' \| 'pink' \| 'tertiary' \| 'mint'` | `'neutral'` | Subtle background tint |
| `shadow` | `'pop' \| 'pop-soft' \| 'pop-pink' \| 'pop-mint' \| 'pop-tertiary' \| 'pop-violet' \| 'none'` | `'pop-soft'` | Hard offset shadow color |
| `kicker` | `string` | — | Short uppercase label above the title |
| `kickerTone` | kicker tone | `'tertiary'` | — |
| `title` | `string \| node` | — | — |
| `action` | `node` | — | Trailing slot in the header row |
| `floatingIcon` | `node` | — | Rendered half-out of the top border — pair with `<FloatingIconBadge>` |
| `wiggleOnHover` | `bool` | `false` | Small rotate + scale on hover |
| `liftOnHover` | `bool` | `true` | Translate up + extend shadow on hover |
| `hideHeader` | `bool` | `false` | Skip the title row entirely |

### StickerButton

Semi `Button` wrapper with painted variants and enforced tap targets.

| Prop | Type | Default |
|---|---|---|
| `variant` | `'primary' \| 'secondary' \| 'ghost' \| 'danger' \| 'icon'` | `'primary'` |
| `size` | `'sm' \| 'md' \| 'lg'` | `'md'` (48px tap target — spec default) |
| `block` | `bool` | `false` |

Every other Semi `Button` prop (`loading`, `disabled`, `icon`, `onClick`,
`htmlType`, …) passes through unchanged.

### PlayfulKicker

The short uppercase eyebrow chip shown above section titles.

```jsx
<PlayfulKicker tone='pink' icon={<Sparkles size={14} strokeWidth={2.5} />}>
  NEW RELEASE
</PlayfulKicker>
```

### SquiggleDivider

Hand-drawn section divider, with an optional centered label.

```jsx
<SquiggleDivider color='accent' label='or' />
```

### DotGridBackdrop

Pointer-events-none dot-grid texture. Drop inside any `position: relative`
parent — typically the app shell `<Content>` for ambient texture.

### ConfettiShapes / ConfettiShape

Absolute-positioned decorative shapes. Always render `<ConfettiShapes>` as the
wrapper so pointer events are ignored and z-index is 0.

```jsx
<div className='relative'>
  <ConfettiShapes>
    <ConfettiShape kind='circle' tone='violet' top='10%' left='8%' />
    <ConfettiShape kind='triangle' tone='pink' size={48} bottom='20%' right='15%' />
  </ConfettiShapes>
  {/* content */}
</div>
```

### FloatingIconBadge

Circle icon with 2px foreground border and a mini hard shadow. Pair with
`StickerCard.floatingIcon` to get the "icon peeking out of the card top"
signature.

### SectionHeader

Standardized page/section heading. Enforces the right fonts, spacing, and
kicker positioning.

```jsx
<SectionHeader
  kicker='DASHBOARD'
  title='Today at a glance'
  subtitle='The metrics that shape your next move.'
  action={<StickerButton size='sm'>Refresh</StickerButton>}
/>
```

### PlayfulTable

`Semi.Table` wrapped with the bordered, uppercase-header table chrome. Pass
every Semi Table prop you already use — nothing else changes.

### PlayfulFormField

Wraps `Form.Input` / `Form.Select` / `Form.TextArea` / etc. to apply:

- uppercase bold Outfit label
- 2px border on the input wrapper
- accent-colored hard-shadow focus ring

```jsx
<PlayfulFormField field='email' label='Email' placeholder='you@example.com' />
<PlayfulFormField as={Form.Select} field='role' label='Role' optionList={...} />
```

### PlayfulModal

Semi `Modal` wrapped with the playful chrome (bordered card, hard shadow,
yellow-tinted header with Outfit title). Pass `tone` to swap header colors.

### PlayfulTag

Chunky pill tag with 2px foreground border and a mini hard shadow. Use
instead of Semi's `Tag` when you want the sticker feel.

### PlayfulEmpty

Empty-state wrapper with a confetti decoration layer and standardized
typography. Pass `decorated={false}` to skip the confetti in denser
contexts.

### PlayfulToastContainer

A drop-in replacement for `react-toastify`'s `ToastContainer`. Mount exactly
once in the app shell; existing `showSuccess` / `showError` / `showInfo`
calls throughout the codebase pick up the styling automatically.

## VChart theme

`src/helpers/vchartTheme.js` registers a `playful-geometric` VChart theme on
app start (via `src/index.jsx`). It rewires:

- 10-color palette anchored on the four playful tones
- Axis / legend / tooltip fonts and colors
- Tooltip panel: 2px bordered white card with a hard shadow

You don't need to import anything in chart call sites — the theme is global.

## DO / DON'T

**DO**

- Compose pages out of primitives: `<SectionHeader>` + `<StickerCard>` +
  `<PlayfulFormField>` + `<StickerButton>`.
- Rotate accent tones on sibling cards to create a confetti rhythm
  (violet → pink → tertiary → mint).
- Use `transition-playful` on any new interactive element.
- Reach for `<PlayfulTag>` / `<PlayfulKicker>` before Semi's `Tag` /
  `Typography.Text`.

**DON'T**

- Don't reintroduce `dark:*` — the app is light-only. The lint guard in
  `web/scripts/check-no-dark.mjs` will fail CI if you do.
- Don't use raw Tailwind `text-gray-*` / `bg-gray-*` / `text-blue-*` / hex
  shadows. Use playful tokens.
- Don't wrap `<StickerCard>` around another `<StickerCard>` — two hard
  shadows stacked look busy. Use a `<StickerCard>` with inner
  `playful-section-card` children instead.
- Don't apply `wiggleOnHover` to cards that contain form inputs or tables —
  the hover rotation disrupts input focus.
- Don't bypass `<StickerButton>` by painting a raw `<Button>` with
  `candy-btn` classes. The primitive exists so tap targets and motion
  remain consistent.

## Adding a new primitive

1. Add the component file under this folder.
2. Add CSS (if needed) to the `Playful Primitives (Phase 1)` section of
   `src/index.css`. Scope with a `.playful-*` class name.
3. Export from `./index.js`.
4. Add a variant demo to `src/pages/PlayfulGallery/index.jsx`.
5. Document the prop table in this README.
