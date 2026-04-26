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
import { Table } from '@douyinfe/semi-ui';

/**
 * PlayfulTable — thin wrapper around Semi's Table that scopes the Playful
 * table styling (bordered, bold uppercase headers, row hover tint, hard shadow).
 *
 * Any Semi Table prop is passed through unchanged.
 */
const PlayfulTable = React.forwardRef(function PlayfulTable(
  { className = '', wrapClassName = '', ...rest },
  ref,
) {
  return (
    <div className={`playful-table-wrap ${wrapClassName}`.trim()}>
      <Table ref={ref} className={className} {...rest} />
    </div>
  );
});

export default PlayfulTable;
