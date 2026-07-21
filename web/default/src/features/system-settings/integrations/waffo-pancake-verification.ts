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
import type { CatalogStore, WaffoPancakeEnvironment } from './waffo-pancake-api'

export type WaffoPancakeVerificationPhase =
  | 'unverified'
  | 'verifying'
  | 'verified'

export interface WaffoPancakeBinding {
  storeID: string
  productID: string
}

interface WaffoPancakeCatalogRequestToken {
  credentialGeneration: number
  requestSerial: number
}

export interface WaffoPancakeAsyncGuard {
  activate: () => void
  dispose: () => void
  captureCredentials: () => number
  invalidateCredentials: () => void
  beginCatalogRequest: (
    credentialGeneration: number
  ) => WaffoPancakeCatalogRequestToken
  isCredentialCurrent: (credentialGeneration: number) => boolean
  isCatalogRequestCurrent: (token: WaffoPancakeCatalogRequestToken) => boolean
  isActive: () => boolean
}

export function createWaffoPancakeAsyncGuard(): WaffoPancakeAsyncGuard {
  let active = true
  let credentialGeneration = 0
  let requestSerial = 0

  return {
    activate() {
      active = true
    },
    dispose() {
      active = false
      credentialGeneration += 1
      requestSerial += 1
    },
    captureCredentials() {
      return credentialGeneration
    },
    invalidateCredentials() {
      credentialGeneration += 1
      requestSerial += 1
    },
    beginCatalogRequest(generation) {
      requestSerial += 1
      return {
        credentialGeneration: generation,
        requestSerial,
      }
    },
    isCredentialCurrent(generation) {
      return active && generation === credentialGeneration
    },
    isCatalogRequestCurrent(token) {
      return (
        active &&
        token.credentialGeneration === credentialGeneration &&
        token.requestSerial === requestSerial
      )
    },
    isActive() {
      return active
    },
  }
}

export function isVerifiedWaffoPancakeEnvironment(
  environment: WaffoPancakeEnvironment
): environment is Exclude<WaffoPancakeEnvironment, ''> {
  return environment === 'test' || environment === 'prod'
}

function hasCatalogBinding(
  stores: CatalogStore[],
  environment: WaffoPancakeEnvironment,
  binding: WaffoPancakeBinding
): boolean {
  if (!binding.storeID || !binding.productID) return false
  const store = filterUsableWaffoPancakeStores(stores, environment).find(
    (item) => item.id === binding.storeID
  )
  return !!store?.onetimeProducts.some((item) => item.id === binding.productID)
}

export function filterUsableWaffoPancakeStores(
  stores: CatalogStore[],
  environment: WaffoPancakeEnvironment
): CatalogStore[] {
  return stores.filter((store) => {
    const status = store.status.trim()
    if (status && status.toLowerCase() !== 'active') return false
    return environment !== 'prod' || store.prodEnabled
  })
}

export function selectWaffoPancakeBinding(
  stores: CatalogStore[],
  environment: WaffoPancakeEnvironment,
  preferredBinding: WaffoPancakeBinding | undefined,
  savedBinding: WaffoPancakeBinding
): WaffoPancakeBinding {
  const usableStores = filterUsableWaffoPancakeStores(stores, environment)

  if (
    preferredBinding &&
    hasCatalogBinding(usableStores, environment, preferredBinding)
  ) {
    return preferredBinding
  }

  if (
    preferredBinding?.storeID &&
    !preferredBinding.productID &&
    usableStores.some((item) => item.id === preferredBinding.storeID)
  ) {
    return preferredBinding
  }

  if (hasCatalogBinding(usableStores, environment, savedBinding)) {
    return savedBinding
  }

  const storeWithProduct = usableStores.find(
    (item) => item.onetimeProducts.length > 0
  )
  if (storeWithProduct) {
    return {
      storeID: storeWithProduct.id,
      productID: storeWithProduct.onetimeProducts[0].id,
    }
  }

  return { storeID: usableStores[0]?.id ?? '', productID: '' }
}

export function canSaveWaffoPancakeConfig(params: {
  phase: WaffoPancakeVerificationPhase
  environment: WaffoPancakeEnvironment
  binding: WaffoPancakeBinding
  catalog: CatalogStore[]
}): boolean {
  return (
    params.phase === 'verified' &&
    isVerifiedWaffoPancakeEnvironment(params.environment) &&
    hasCatalogBinding(params.catalog, params.environment, params.binding)
  )
}
