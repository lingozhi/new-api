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
import ConfettiShapes, { ConfettiShape } from './ConfettiShapes';

/**
 * PlayfulEmpty — empty state wrapper with playful decorations.
 *
 * Props:
 *   kicker       — optional eyebrow label
 *   title        — string or node
 *   description  — string or node
 *   illustration — optional node rendered above the title (e.g. an icon in a
 *                  FloatingIconBadge)
 *   action       — trailing node (button) below the description
 *   decorated    — whether to render a confetti cluster behind the content
 *                  (default true)
 */
const PlayfulEmpty = ({
  kicker,
  title,
  description,
  illustration,
  action,
  decorated = true,
  className = '',
}) => (
  <div className={`playful-empty ${className}`.trim()}>
    {decorated ? (
      <ConfettiShapes>
        <ConfettiShape kind='circle' tone='tertiary' size={36} top='10%' left='12%' rotate={-8} />
        <ConfettiShape kind='square' tone='pink' size={28} top='20%' right='14%' rotate={10} />
        <ConfettiShape kind='triangle' tone='mint' size={32} bottom='18%' left='20%' />
        <ConfettiShape kind='blob' tone='violet' size={44} bottom='12%' right='18%' rotate={-4} />
      </ConfettiShapes>
    ) : null}
    <div className='relative z-10 flex flex-col items-center gap-3'>
      {illustration ? <div className='mb-1'>{illustration}</div> : null}
      {kicker ? <PlayfulKicker>{kicker}</PlayfulKicker> : null}
      {title ? (
        typeof title === 'string' ? (
          <h3 className='playful-empty__title'>{title}</h3>
        ) : (
          title
        )
      ) : null}
      {description ? (
        typeof description === 'string' ? (
          <p className='playful-empty__description'>{description}</p>
        ) : (
          description
        )
      ) : null}
      {action ? <div className='mt-2'>{action}</div> : null}
    </div>
  </div>
);

export default PlayfulEmpty;
