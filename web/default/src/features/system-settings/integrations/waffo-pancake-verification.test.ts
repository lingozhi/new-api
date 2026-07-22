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

import type { CatalogStore } from './waffo-pancake-api'
import {
  canSaveWaffoPancakeConfig,
  createWaffoPancakeAsyncGuard,
  isVerifiedWaffoPancakeEnvironment,
  selectWaffoPancakeBinding,
} from './waffo-pancake-verification'

function createDeferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

const catalog: CatalogStore[] = [
  {
    id: 'store-a',
    name: 'Store A',
    status: 'active',
    prodEnabled: true,
    onetimeProducts: [{ id: 'product-a', name: 'Product A', status: 'active' }],
  },
  {
    id: 'store-b',
    name: 'Store B',
    status: 'active',
    prodEnabled: true,
    onetimeProducts: [{ id: 'product-b', name: 'Product B', status: 'active' }],
  },
]

describe('Waffo Pancake verification contract', () => {
  test('treats an empty legacy environment as unverified', () => {
    assert.equal(isVerifiedWaffoPancakeEnvironment(''), false)
    assert.equal(isVerifiedWaffoPancakeEnvironment('test'), true)
    assert.equal(isVerifiedWaffoPancakeEnvironment('prod'), true)
  })

  test('selects only bindings present in the verified catalog', () => {
    assert.deepEqual(
      selectWaffoPancakeBinding(
        catalog,
        'prod',
        { storeID: 'store-b', productID: 'missing-product' },
        { storeID: 'store-a', productID: 'product-a' }
      ),
      { storeID: 'store-a', productID: 'product-a' }
    )
  })

  test('falls back to the first complete catalog binding', () => {
    assert.deepEqual(
      selectWaffoPancakeBinding(
        catalog,
        'prod',
        { storeID: 'missing-store', productID: 'missing-product' },
        { storeID: 'missing-store', productID: 'missing-product' }
      ),
      { storeID: 'store-a', productID: 'product-a' }
    )
  })

  test('requires verified credentials, a detected environment, and a complete binding', () => {
    const binding = { storeID: 'store-a', productID: 'product-a' }

    assert.equal(
      canSaveWaffoPancakeConfig({
        phase: 'verified',
        environment: 'prod',
        binding,
        catalog,
      }),
      true
    )
    assert.equal(
      canSaveWaffoPancakeConfig({
        phase: 'verifying',
        environment: 'prod',
        binding,
        catalog,
      }),
      false
    )
    assert.equal(
      canSaveWaffoPancakeConfig({
        phase: 'verified',
        environment: '',
        binding,
        catalog,
      }),
      false
    )
    assert.equal(
      canSaveWaffoPancakeConfig({
        phase: 'verified',
        environment: 'test',
        binding: { storeID: 'store-a', productID: '' },
        catalog,
      }),
      false
    )
  })

  test('excludes inactive and production-disabled stores from bindings', () => {
    const stores: CatalogStore[] = [
      {
        ...catalog[0],
        id: 'inactive-store',
        status: 'disabled',
      },
      {
        ...catalog[1],
        id: 'test-only-store',
        prodEnabled: false,
      },
    ]

    assert.deepEqual(
      selectWaffoPancakeBinding(stores, 'prod', undefined, {
        storeID: '',
        productID: '',
      }),
      { storeID: '', productID: '' }
    )
    assert.equal(
      canSaveWaffoPancakeConfig({
        phase: 'verified',
        environment: 'prod',
        binding: { storeID: 'test-only-store', productID: 'product-b' },
        catalog: stores,
      }),
      false
    )
    assert.deepEqual(
      selectWaffoPancakeBinding(stores, 'test', undefined, {
        storeID: '',
        productID: '',
      }),
      { storeID: 'test-only-store', productID: 'product-b' }
    )
  })

  test('rejects deferred pair work after credential edits or unmount', async () => {
    for (const invalidate of ['credentials', 'unmount'] as const) {
      const guard = createWaffoPancakeAsyncGuard()
      const credentialGeneration = guard.captureCredentials()
      const deferred = createDeferred<void>()
      let committed = false
      const pairWork = (async () => {
        await deferred.promise
        if (!guard.isCredentialCurrent(credentialGeneration)) return
        committed = true
      })()

      if (invalidate === 'credentials') {
        guard.invalidateCredentials()
      } else {
        guard.dispose()
      }
      deferred.resolve()
      await pairWork

      assert.equal(committed, false, invalidate)
    }
  })
})
