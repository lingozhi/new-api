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

const TONE_BG = {
  tertiary: 'bg-playful-tertiary text-playful-foreground',
  pink: 'bg-playful-secondary text-white',
  violet: 'bg-playful-accent text-white',
  mint: 'bg-playful-quaternary text-playful-foreground',
  neutral: 'bg-playful-card text-playful-foreground',
};

/**
 * PlayfulKicker — the eyebrow chip that sits above a section heading.
 *
 * Props:
 *   tone     — 'tertiary' (yellow, default) | 'pink' | 'violet' | 'mint' | 'neutral'
 *   icon     — optional leading node (e.g. a Lucide icon)
 *   children — the short, all-caps label
 */
const PlayfulKicker = ({ tone = 'tertiary', icon, children, className = '' }) => {
  const toneClass = TONE_BG[tone] || TONE_BG.tertiary;
  return (
    <span className={`playful-kicker ${toneClass} ${className}`.trim()}>
      {icon ? <span className='inline-flex items-center'>{icon}</span> : null}
      <span>{children}</span>
    </span>
  );
};

export default PlayfulKicker;
