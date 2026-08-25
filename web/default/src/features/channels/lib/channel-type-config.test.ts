/*
Copyright (C) 2023-2026 QuantumNous

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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getRelatedModelsForChannelType } from './channel-type-config'

describe('channel type model presets', () => {
  test('uses AutoDL supported models even when they are absent from the catalog', () => {
    assert.deepEqual(
      getRelatedModelsForChannelType(60, ['gpt-5', 'claude-sonnet']),
      ['MiniMax-H3']
    )
  })

  test('keeps the OpenAI preset limited to matching catalog models', () => {
    assert.deepEqual(
      getRelatedModelsForChannelType(1, ['gpt-5', 'text-embedding-3', 'o3']),
      ['gpt-5', 'text-embedding-3']
    )
  })
})
