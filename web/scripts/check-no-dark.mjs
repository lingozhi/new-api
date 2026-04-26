#!/usr/bin/env node
/*
 * Playful Geometric — guard against Tailwind `dark:` variants and dark-mode CSS
 * re-creeping into the codebase after Phase 0.
 *
 * Fails the build if it finds any of:
 *   - `dark:` Tailwind variant in .js/.jsx/.ts/.tsx
 *   - `html.dark`, `.dark-mode`, `.theme-dark`, `body.dark`, or
 *     `prefers-color-scheme` selectors in .css/.scss
 *
 * Wired into `bun run lint` so it runs locally and in CI.
 */

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { extname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = join(fileURLToPath(new URL('.', import.meta.url)), '..', 'src');
const JS_EXTS = new Set(['.js', '.jsx', '.ts', '.tsx']);
const CSS_EXTS = new Set(['.css', '.scss']);

const JS_PATTERN = /\bdark:[A-Za-z0-9_\-/[\]()#.%]+/g;
const CSS_PATTERN =
  /html\.dark\b|\bbody\.dark\b|\.dark-mode\b|\.theme-dark\b|\.dark\s*(?=[.#:\s{>+~])|@media[^{]*prefers-color-scheme/g;

const violations = [];

function walk(dir) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    const info = statSync(full);
    if (info.isDirectory()) {
      walk(full);
      continue;
    }
    const ext = extname(full);
    let pattern;
    if (JS_EXTS.has(ext)) pattern = JS_PATTERN;
    else if (CSS_EXTS.has(ext)) pattern = CSS_PATTERN;
    else continue;

    const content = readFileSync(full, 'utf8');
    let m;
    pattern.lastIndex = 0;
    while ((m = pattern.exec(content)) !== null) {
      const line = content.slice(0, m.index).split('\n').length;
      violations.push({
        file: relative(join(ROOT, '..'), full),
        line,
        match: m[0],
      });
    }
  }
}

walk(ROOT);

if (violations.length > 0) {
  console.error(
    `\n✗ Playful Geometric guard: found ${violations.length} dark-mode reference(s).\n`,
  );
  for (const v of violations) {
    console.error(`  ${v.file}:${v.line}  →  ${v.match}`);
  }
  console.error(
    '\nDark mode was removed in Phase 0 of the Playful Geometric migration.\n' +
      'See CLAUDE.md §Rule 7 for the allowed playful-* token set.\n',
  );
  process.exit(1);
}

console.log('✓ No dark-mode references found.');
