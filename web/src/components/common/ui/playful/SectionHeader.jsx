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

/**
 * SectionHeader — standardized page/section heading with optional kicker and
 * trailing action slot.
 *
 * Props:
 *   kicker     — short uppercase label shown in a PlayfulKicker above the title
 *   kickerTone — tone passed through to the kicker
 *   title      — string or node (required for visual grammar)
 *   subtitle   — secondary line below the title
 *   action     — trailing node (button, link, filter)
 *   as         — heading level: 'h1' | 'h2' | 'h3' (default 'h2')
 */
const SectionHeader = ({
  kicker,
  kickerTone,
  kickerIcon,
  title,
  subtitle,
  action,
  as = 'h2',
  className = '',
  titleClassName = '',
  subtitleClassName = '',
}) => {
  const HeadingTag = as;
  return (
    <div className={`playful-section-header ${className}`.trim()}>
      <div className='playful-section-header__text'>
        {kicker ? (
          <PlayfulKicker tone={kickerTone} icon={kickerIcon}>
            {kicker}
          </PlayfulKicker>
        ) : null}
        {title ? (
          typeof title === 'string' ? (
            <HeadingTag className={`playful-section-header__title ${titleClassName}`.trim()}>
              {title}
            </HeadingTag>
          ) : (
            title
          )
        ) : null}
        {subtitle ? (
          typeof subtitle === 'string' ? (
            <p className={`playful-section-header__subtitle ${subtitleClassName}`.trim()}>
              {subtitle}
            </p>
          ) : (
            subtitle
          )
        ) : null}
      </div>
      {action ? <div className='flex shrink-0 items-center gap-2'>{action}</div> : null}
    </div>
  );
};

export default SectionHeader;
