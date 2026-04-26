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
import { Form } from '@douyinfe/semi-ui';

/**
 * PlayfulFormField — wraps any Semi Form.* component (Input, Select,
 * TextArea, InputNumber, DatePicker, etc.) to apply the Playful label and
 * focused hard-shadow styling.
 *
 * Props:
 *   as         — the Semi Form subcomponent (default Form.Input). Pass e.g.
 *                `as={Form.Select}` to render a select instead.
 *   className  — merged onto the inner Semi component.
 *   fieldClassName — applied to the outer wrapper div.
 *
 * All other props pass straight to the Semi component.
 */
const PlayfulFormField = React.forwardRef(function PlayfulFormField(
  {
    as: Component = Form.Input,
    className = '',
    fieldClassName = '',
    ...rest
  },
  ref,
) {
  return (
    <div className={`playful-form-field ${fieldClassName}`.trim()}>
      <Component ref={ref} className={className} {...rest} />
    </div>
  );
});

export default PlayfulFormField;
