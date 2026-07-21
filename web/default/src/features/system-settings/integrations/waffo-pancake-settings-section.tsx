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
import * as React from 'react'
import type { SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import { removeTrailingSlash } from './utils'
import {
  type CatalogStore,
  type PairOrphanError,
  type PairResult,
  type WaffoPancakeEnvironment,
  createWaffoPancakePair,
  listWaffoPancakeCatalog,
} from './waffo-pancake-api'
import {
  type WaffoPancakeBinding,
  type WaffoPancakeVerificationPhase,
  createWaffoPancakeAsyncGuard,
  filterUsableWaffoPancakeStores,
  isVerifiedWaffoPancakeEnvironment,
  selectWaffoPancakeBinding,
} from './waffo-pancake-verification'

export type WaffoPancakeSettingsValues = {
  WaffoPancakeMerchantID: string
  WaffoPancakePrivateKey: string
  WaffoPancakeReturnURL: string
  WaffoPancakeEnvironment: WaffoPancakeEnvironment
}

interface Props {
  defaultValues: WaffoPancakeSettingsValues
  values: WaffoPancakeSettingsValues
  onValueChange: <K extends keyof WaffoPancakeSettingsValues>(
    key: K,
    value: WaffoPancakeSettingsValues[K]
  ) => void
  catalog: CatalogStore[]
  onCatalogChange: (catalog: CatalogStore[]) => void
  selectedBinding: WaffoPancakeBinding
  savedBinding: WaffoPancakeBinding
  onSelectedBindingChange: (value: SetStateAction<WaffoPancakeBinding>) => void
  verificationPhase: WaffoPancakeVerificationPhase
  onVerificationPhaseChange: (value: WaffoPancakeVerificationPhase) => void
}

const PANCAKE_DASHBOARD_URL = 'https://pancake.waffo.ai/merchant/dashboard'
const DEFAULT_NEW_STORE_NAME = 'new-api-store'
const DEFAULT_NEW_PRODUCT_NAME = 'new-api-charge-product'
const DEFAULT_NEW_PAIR_NAME = `${DEFAULT_NEW_STORE_NAME} + ${DEFAULT_NEW_PRODUCT_NAME}`

export function WaffoPancakeSettingsSection({
  defaultValues,
  values,
  onValueChange,
  catalog,
  onCatalogChange,
  selectedBinding,
  savedBinding,
  onSelectedBindingChange,
  verificationPhase,
  onVerificationPhaseChange,
}: Props) {
  const { t } = useTranslation()

  const [creatingPair, setCreatingPair] = React.useState(false)
  const chosenStoreID = selectedBinding.storeID
  const chosenProductID = selectedBinding.productID
  const storeID = savedBinding.storeID
  const productID = savedBinding.productID
  const returnURL = values.WaffoPancakeReturnURL
  const [asyncGuard] = React.useState(createWaffoPancakeAsyncGuard)
  const savedBindingRef = React.useRef(savedBinding)
  const didScheduleVerificationRef = React.useRef(false)

  React.useEffect(() => {
    asyncGuard.activate()
    return () => asyncGuard.dispose()
  }, [asyncGuard])

  React.useEffect(() => {
    savedBindingRef.current = savedBinding
  }, [savedBinding])

  const usableCatalog = React.useMemo(
    () =>
      filterUsableWaffoPancakeStores(catalog, values.WaffoPancakeEnvironment),
    [catalog, values.WaffoPancakeEnvironment]
  )

  const productsForChosenStore = React.useMemo(() => {
    if (!chosenStoreID) return []
    return (
      usableCatalog.find((s) => s.id === chosenStoreID)?.onetimeProducts ?? []
    )
  }, [chosenStoreID, usableCatalog])

  const storeSelectItems = React.useMemo(() => {
    return usableCatalog.map((s) => ({
      value: s.id,
      label: `${s.name} (${s.id})`,
    }))
  }, [usableCatalog])
  const productSelectItems = React.useMemo(() => {
    return productsForChosenStore.map((p) => ({
      value: p.id,
      label: `${p.name} (${p.id})`,
    }))
  }, [productsForChosenStore])

  const resetVerification = React.useCallback(() => {
    onCatalogChange([])
    onVerificationPhaseChange('unverified')
  }, [onCatalogChange, onVerificationPhaseChange])

  const invalidateVerification = React.useCallback(() => {
    asyncGuard.invalidateCredentials()
    resetVerification()
  }, [asyncGuard, resetVerification])

  const invalidateCredentials = React.useCallback(() => {
    invalidateVerification()
    onValueChange('WaffoPancakeEnvironment', '')
    onSelectedBindingChange({ storeID: '', productID: '' })
  }, [invalidateVerification, onSelectedBindingChange, onValueChange])

  // Verifies typed creds against Pancake (via /catalog) and refreshes the
  // dropdown options. The API-key environment returned by the catalog is the
  // only environment accepted by the form.
  const verifyAndFetchCatalog = React.useCallback(
    async (
      merchantID: string,
      privateKey: string,
      preselect?: { storeID?: string; productID?: string },
      credentialGeneration = asyncGuard.captureCredentials()
    ) => {
      if (!asyncGuard.isCredentialCurrent(credentialGeneration)) return false
      const requestToken = asyncGuard.beginCatalogRequest(credentialGeneration)
      onCatalogChange([])
      onVerificationPhaseChange('verifying')

      try {
        const body = await listWaffoPancakeCatalog({
          merchantID,
          privateKey,
        })
        if (!asyncGuard.isCatalogRequestCurrent(requestToken)) return false
        if (
          body?.message === 'success' &&
          typeof body.data === 'object' &&
          body.data &&
          isVerifiedWaffoPancakeEnvironment(body.data.environment)
        ) {
          const stores = Array.isArray(body.data.stores) ? body.data.stores : []
          const preferredBinding = preselect
            ? {
                storeID: preselect.storeID ?? '',
                productID: preselect.productID ?? '',
              }
            : undefined
          const nextBinding = selectWaffoPancakeBinding(
            stores,
            body.data.environment,
            preferredBinding,
            savedBindingRef.current
          )

          onCatalogChange(stores)
          onValueChange('WaffoPancakeEnvironment', body.data.environment)
          onSelectedBindingChange(nextBinding)
          onVerificationPhaseChange('verified')
          return true
        } else {
          const reason = typeof body?.data === 'string' ? body.data : undefined
          toast.error(
            reason
              ? `${t('Credentials verification failed')}: ${reason}`
              : t(
                  'Credentials verification failed — double-check Merchant ID and API private key.'
                )
          )
          resetVerification()
          return false
        }
      } catch (err) {
        if (!asyncGuard.isCatalogRequestCurrent(requestToken)) return false
        toast.error(
          `${t('Credentials verification failed')}: ${
            err instanceof Error ? err.message : String(err)
          }`
        )
        resetVerification()
        return false
      }
    },
    [
      asyncGuard,
      onCatalogChange,
      onSelectedBindingChange,
      onValueChange,
      onVerificationPhaseChange,
      resetVerification,
      t,
    ]
  )

  const watchedMerchantID = values.WaffoPancakeMerchantID || ''
  const watchedPrivateKey = values.WaffoPancakePrivateKey || ''
  React.useEffect(() => {
    const isInitialVerification = !didScheduleVerificationRef.current
    didScheduleVerificationRef.current = true
    invalidateVerification()

    const m = watchedMerchantID.trim()
    const k = watchedPrivateKey.trim()
    const savedMerchantID = defaultValues.WaffoPancakeMerchantID.trim()
    const useSavedCredentials = m === savedMerchantID && !k
    if (!useSavedCredentials && (!m || !k)) return

    const credentialGeneration = asyncGuard.captureCredentials()
    onVerificationPhaseChange('verifying')
    const timer = window.setTimeout(
      () => {
        void verifyAndFetchCatalog(
          useSavedCredentials ? '' : m,
          useSavedCredentials ? '' : k,
          undefined,
          credentialGeneration
        )
      },
      isInitialVerification ? 0 : 800
    )
    return () => window.clearTimeout(timer)
  }, [
    watchedMerchantID,
    watchedPrivateKey,
    asyncGuard,
    defaultValues.WaffoPancakeMerchantID,
    invalidateVerification,
    onVerificationPhaseChange,
    verifyAndFetchCatalog,
  ])

  // Returns typed creds when the operator edited either field; otherwise
  // blanks so the backend falls back to persisted creds. Without this,
  // returning admins (saved merchant ID but empty key field) would send
  // a mixed-state body that the backend rejects.
  const readCreds = () => {
    const formMerchant = (values.WaffoPancakeMerchantID || '').trim()
    const formKey = (values.WaffoPancakePrivateKey || '').trim()
    const saved = (defaultValues.WaffoPancakeMerchantID || '').trim()
    const edited = formMerchant !== saved || formKey.length > 0
    if (!edited) return { merchantID: '', privateKey: '' }
    return { merchantID: formMerchant, privateKey: formKey }
  }

  // The minted product's SuccessURL is pinned to the current Return URL
  // field, so we prompt before creating when that field is empty.
  const handleCreatePair = async () => {
    if (!credsReady) {
      toast.error(
        t('Fill in both Merchant ID and API Private Key before creating.')
      )
      return
    }
    if (
      verificationPhase !== 'verified' ||
      !isVerifiedWaffoPancakeEnvironment(values.WaffoPancakeEnvironment)
    ) {
      toast.error(
        t(
          'Credentials verification failed — double-check Merchant ID and API private key.'
        )
      )
      return
    }
    const { merchantID, privateKey } = readCreds()
    const credentialGeneration = asyncGuard.captureCredentials()
    const trimmedReturn = removeTrailingSlash(returnURL.trim())
    if (!trimmedReturn) {
      if (
        !window.confirm(
          t(
            'Payment return URL is empty. Create the product without a SuccessURL redirect?'
          )
        )
      ) {
        return
      }
    }
    setCreatingPair(true)
    try {
      const body = await createWaffoPancakePair({
        merchantID,
        privateKey,
        returnURL: trimmedReturn,
        environment: values.WaffoPancakeEnvironment,
      })
      if (!asyncGuard.isCredentialCurrent(credentialGeneration)) return
      if (
        body?.message === 'success' &&
        typeof body.data === 'object' &&
        body.data
      ) {
        const created = body.data as PairResult
        // Refetch from GraphQL rather than trusting the response body so the
        // dropdowns reflect authoritative state, then anchor on minted IDs.
        await verifyAndFetchCatalog(
          merchantID,
          privateKey,
          {
            storeID: created.store_id,
            productID: created.product_id,
          },
          credentialGeneration
        )
        if (!asyncGuard.isCredentialCurrent(credentialGeneration)) return
        toast.success(
          `${t('Store + product created')}: ${created.store_id} / ${created.product_id}`
        )
        return
      }
      const errData =
        body && typeof body.data === 'object' && body.data !== null
          ? (body.data as PairOrphanError)
          : null
      if (errData?.orphan_store && errData.store_id) {
        await verifyAndFetchCatalog(
          merchantID,
          privateKey,
          {
            storeID: errData.store_id,
            productID: '',
          },
          credentialGeneration
        )
        if (!asyncGuard.isCredentialCurrent(credentialGeneration)) return
      }
      const reason =
        errData?.error ??
        (typeof body?.data === 'string' ? body.data : undefined)
      toast.error(
        reason ? `${t('Creation failed')}: ${reason}` : t('Creation failed')
      )
    } catch (err) {
      if (!asyncGuard.isCredentialCurrent(credentialGeneration)) return
      toast.error(
        `${t('Creation failed')}: ${err instanceof Error ? err.message : String(err)}`
      )
    } finally {
      if (asyncGuard.isActive()) setCreatingPair(false)
    }
  }

  const verifying = verificationPhase === 'verifying'
  const verified = verificationPhase === 'verified'

  // "Not edited" = MerchantID unchanged AND PrivateKey field blank, in
  // which case the backend falls back to persisted creds. Otherwise we
  // require both fields filled (mixed states would fail signature check).
  const savedMerchantID = (defaultValues.WaffoPancakeMerchantID || '').trim()
  const formMerchantID = watchedMerchantID.trim()
  const formPrivateKey = watchedPrivateKey.trim()
  const credsEdited =
    formMerchantID !== savedMerchantID || formPrivateKey.length > 0
  const hasSavedCreds = savedMerchantID.length > 0
  const credsReady = credsEdited
    ? formMerchantID.length > 0 && formPrivateKey.length > 0
    : hasSavedCreds
  const hasCatalog = usableCatalog.length > 0

  let bindStatusMessage: string
  if (!credsReady) {
    bindStatusMessage = t('Fill in the credentials above to begin.')
  } else if (verifying) {
    bindStatusMessage = t(
      'Verifying credentials and pulling stores from your Pancake account...'
    )
  } else if (verified && hasCatalog) {
    bindStatusMessage = t(
      'Mint a fresh pair below — or pick an existing one further down. Click Save when ready.'
    )
  } else if (verified) {
    bindStatusMessage = t(
      'No stores on this merchant yet. Set a return URL and click Create to mint your first pair.'
    )
  } else {
    bindStatusMessage = t(
      'Credentials verification failed — double-check Merchant ID and API private key.'
    )
  }

  let environmentLabel = t('Unknown')
  if (verified && values.WaffoPancakeEnvironment === 'test') {
    environmentLabel = t('Test Mode')
  } else if (verified && values.WaffoPancakeEnvironment === 'prod') {
    environmentLabel = t('Production Mode')
  }

  return (
    <div className='space-y-4 pt-4'>
      <div>
        <h3 className='text-lg font-medium'>{t('Waffo Pancake MoR')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Start collecting payments globally without registering a company. Built for indie developers, OPC sole proprietorships, and startups. Waffo Pancake acts as your Merchant of Record, taking on the compliance burden of global payment collection — consumption tax, invoicing, subscription management, refunds, and chargebacks. Solo developers can launch fast and stay focused on product instead of compliance. Onboard in minutes — one prompt to a full integration.'
          )}
        </p>
      </div>
      <div className='grid min-w-0 gap-x-5 gap-y-4 lg:grid-cols-2'>
        {/* Blue box — webhook configuration only. */}
        <div className='rounded-md bg-blue-50 p-4 text-sm text-blue-900 lg:col-span-2 dark:bg-blue-950 dark:text-blue-100'>
          <p className='mb-2 font-medium'>{t('Webhook Configuration:')}</p>
          <ul className='list-inside list-disc space-y-1'>
            <li>
              {t('Webhook URL (Test):')}{' '}
              <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                {'<ServerAddress>/api/waffo-pancake/webhook/test'}
              </code>
            </li>
            <li>
              {t('Webhook URL (Production):')}{' '}
              <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                {'<ServerAddress>/api/waffo-pancake/webhook/prod'}
              </code>
            </li>
            <li>
              {t(
                'Register each URL into the matching Test Mode / Production Mode webhook slot in the Pancake dashboard. Separate endpoints prevent test traffic from accidentally crediting production accounts.'
              )}
            </li>
            <li>
              {t('Configure at:')}{' '}
              <a
                href={PANCAKE_DASHBOARD_URL}
                target='_blank'
                rel='noreferrer'
                className='underline hover:no-underline'
              >
                {t('Waffo Pancake Dashboard')}
              </a>
            </li>
          </ul>
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='waffo-pancake-environment'>{t('Environment')}</Label>
          <Input
            id='waffo-pancake-environment'
            value={environmentLabel}
            readOnly
            aria-readonly='true'
            className='bg-muted'
          />
        </div>

        <div className='grid gap-1.5'>
          <Label>{t('Merchant ID')}</Label>
          <Input
            placeholder='MER_xxx'
            autoComplete='off'
            value={values.WaffoPancakeMerchantID}
            onChange={(event) => {
              invalidateCredentials()
              onValueChange('WaffoPancakeMerchantID', event.target.value)
            }}
          />
        </div>

        <div className='grid gap-1.5 lg:col-span-2'>
          <Label>{t('API Private Key')}</Label>
          <Textarea
            rows={4}
            placeholder={t('Leave blank to keep the existing key')}
            autoComplete='new-password'
            value={values.WaffoPancakePrivateKey}
            onChange={(event) => {
              invalidateCredentials()
              onValueChange('WaffoPancakePrivateKey', event.target.value)
            }}
            className='font-mono text-xs'
          />
        </div>

        {/*
          Binding section — split into two visually distinct paths:
          (A) "Use existing" pair from the loaded catalog — only rendered when
              the merchant actually has stores, so first-time setup isn't
              cluttered by dead dropdowns.
          (B) "Create a fresh pair" — always available, paired with the
              return URL field that's only meaningful here.
          The two paths are split by an "or" divider so the operator never has
          to wonder which field belongs to which intent.
        */}
        <div className='space-y-4 pt-2 lg:col-span-2'>
          <div>
            <h4 className='font-medium'>
              {t('Bind a Pancake store + product')}
            </h4>
            <p className='text-muted-foreground text-xs' aria-live='polite'>
              {bindStatusMessage}
            </p>
          </div>

          {/*
              Operator-facing explainer: why only ONE store + product needs
              to be bound at the gateway level, and what each piece is used
              for. Subscriptions reuse the same Store but get their own
              per-plan product, configured in the Subscriptions admin.
            */}
          <div className='rounded-md border border-blue-200 bg-blue-50 p-3 text-xs text-blue-900 dark:border-blue-900/60 dark:bg-blue-950/40 dark:text-blue-100'>
            <p className='mb-1 font-medium'>
              {t('Why only one store + product?')}
            </p>
            <ul className='list-inside list-disc space-y-1'>
              <li>
                {t(
                  'The bound Store is the parent container for every Pancake product new-api creates from this admin — both the wallet top-up product and any subscription-plan products. One store is enough; pin a different one only if you genuinely run separate Pancake catalogs.'
                )}
              </li>
              <li>
                {t(
                  'The bound Product powers wallet top-ups: when a user enters any amount, new-api runs the checkout against this single Pancake product and overrides the price per session — no need to pre-create $1 / $5 / $10 SKUs.'
                )}
              </li>
              <li>
                {t(
                  'Subscription plans do NOT use the bound Product — each plan has its own dedicated Pancake product, set in the Subscriptions admin (or auto-minted via the "+ Create" button there).'
                )}
              </li>
            </ul>
          </div>

          {/* Create section — first, since creating auto-fills the pick-existing dropdowns below. */}
          <div className='space-y-1.5'>
            <Label>{t('Payment return URL')}</Label>
            <div className='flex gap-2'>
              <Input
                placeholder='https://example.com/console/topup'
                value={returnURL}
                onChange={(event) =>
                  onValueChange('WaffoPancakeReturnURL', event.target.value)
                }
                className='flex-1'
              />
              <Button
                type='button'
                variant='outline'
                onClick={handleCreatePair}
                disabled={
                  creatingPair ||
                  verifying ||
                  !credsReady ||
                  !isVerifiedWaffoPancakeEnvironment(
                    values.WaffoPancakeEnvironment
                  )
                }
                className='shrink-0'
              >
                {creatingPair
                  ? t('Creating...')
                  : `+ ${t('Create')} ${DEFAULT_NEW_PAIR_NAME}`}
              </Button>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t(
                "Used as SuccessURL on the new product. You'll be prompted to confirm if left blank."
              )}
            </p>
          </div>

          {hasCatalog ? (
            <>
              <div className='relative flex items-center py-1'>
                <div className='flex-1 border-t' />
                <span className='text-muted-foreground px-3 text-[10px] font-medium tracking-[0.2em] uppercase'>
                  {t('or pick existing')}
                </span>
                <div className='flex-1 border-t' />
              </div>

              <div className='grid grid-cols-2 gap-3'>
                <div className='grid gap-1.5'>
                  <Label>{t('Store')}</Label>
                  <Select
                    items={storeSelectItems}
                    value={chosenStoreID}
                    onValueChange={(value) => {
                      // Base UI Select can deliver null on deselect.
                      onSelectedBindingChange({
                        storeID: value ?? '',
                        productID: '',
                      })
                    }}
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue placeholder={t('Select a store')} />
                    </SelectTrigger>
                    <SelectContent>
                      {storeSelectItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className='grid gap-1.5'>
                  <Label>{t('Product')}</Label>
                  <Select
                    items={productSelectItems}
                    value={chosenProductID}
                    onValueChange={(value) =>
                      onSelectedBindingChange((previous) => ({
                        ...previous,
                        productID: value ?? '',
                      }))
                    }
                    disabled={!chosenStoreID || productSelectItems.length === 0}
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue placeholder={t('Select a product')} />
                    </SelectTrigger>
                    <SelectContent>
                      {productSelectItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </>
          ) : null}

          <div className='flex items-center gap-3'>
            {storeID || productID ? (
              <div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs'>
                {storeID ? (
                  <span>
                    {t('Bound store:')}{' '}
                    <code className='bg-muted rounded px-1 py-0.5'>
                      {storeID}
                    </code>
                  </span>
                ) : null}
                {productID ? (
                  <span>
                    {t('Bound product:')}{' '}
                    <code className='bg-muted rounded px-1 py-0.5'>
                      {productID}
                    </code>
                  </span>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  )
}
