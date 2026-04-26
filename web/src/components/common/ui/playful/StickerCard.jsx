/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React from 'react';
import PlayfulKicker from './PlayfulKicker';

const TONES = new Set(['neutral', 'violet', 'pink', 'tertiary', 'mint']);
const SHADOWS = new Set([
  'pop',
  'pop-soft',
  'pop-pink',
  'pop-mint',
  'pop-tertiary',
  'pop-violet',
  'none',
]);

/**
 * StickerCard — canonical Playful Geometric container.
 *
 * Pattern: 2px foreground border + hard offset shadow + optional floating
 * icon peeking out of the top border + optional eyebrow kicker + optional
 * hover wiggle.
 *
 * Props:
 *   tone          — 'neutral' | 'violet' | 'pink' | 'tertiary' | 'mint'
 *                   Tints the card background subtly. Default: 'neutral'.
 *   shadow        — 'pop' | 'pop-soft' | 'pop-pink' | 'pop-mint' |
 *                   'pop-tertiary' | 'pop-violet' | 'none'. Default: 'pop-soft'.
 *   kicker        — string rendered as a PlayfulKicker above the title.
 *   kickerTone    — tone passed through to the kicker.
 *   title         — string or node shown as the card title.
 *   action        — trailing node (e.g. button) in the header row.
 *   floatingIcon  — React node rendered as a FloatingIconBadge-style badge
 *                   peeking out of the top border of the card.
 *   wiggleOnHover — slight rotate+scale on hover. Default: false.
 *   liftOnHover   — translate up + extend shadow on hover. Default: true.
 *   hideHeader    — suppresses the title row entirely.
 *   bodyClassName, headerClassName, className — escape hatches.
 */
const StickerCard = React.forwardRef(function StickerCard(
  {
    tone = 'neutral',
    shadow = 'pop-soft',
    kicker,
    kickerTone,
    kickerIcon,
    title,
    action,
    floatingIcon,
    wiggleOnHover = false,
    liftOnHover = true,
    hideHeader = false,
    bodyClassName = '',
    headerClassName = '',
    className = '',
    style,
    children,
    onClick,
    ...rest
  },
  ref,
) {
  const toneClass = `playful-sticker-card--tone-${TONES.has(tone) ? tone : 'neutral'}`;
  const shadowClass = `playful-sticker-card--shadow-${SHADOWS.has(shadow) ? shadow : 'pop-soft'}`;
  const wiggleClass = wiggleOnHover ? 'playful-sticker-card--wiggle' : '';
  const liftClass = liftOnHover ? 'playful-sticker-card--lift' : '';

  const showHeader = !hideHeader && (title || action || kicker);

  return (
    <div
      ref={ref}
      className={`playful-sticker-card ${toneClass} ${shadowClass} ${wiggleClass} ${liftClass} ${className}`.trim()}
      style={style}
      onClick={onClick}
      {...rest}
    >
      {floatingIcon ? (
        <div className='playful-sticker-card__floating-icon'>{floatingIcon}</div>
      ) : null}

      {showHeader ? (
        <div className={`playful-sticker-card__header ${headerClassName}`.trim()}>
          <div className='flex min-w-0 flex-col gap-1.5'>
            {kicker ? (
              <PlayfulKicker tone={kickerTone} icon={kickerIcon}>
                {kicker}
              </PlayfulKicker>
            ) : null}
            {title ? (
              typeof title === 'string' ? (
                <h3 className='playful-sticker-card__title truncate'>{title}</h3>
              ) : (
                title
              )
            ) : null}
          </div>
          {action ? <div className='flex shrink-0 items-center gap-2'>{action}</div> : null}
        </div>
      ) : null}

      <div className={`playful-sticker-card__body ${bodyClassName}`.trim()}>{children}</div>
    </div>
  );
});

export default StickerCard;
