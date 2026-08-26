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
import {
  ChevronRight,
  Copy,
  Gauge,
  KeyRound,
  ScrollText,
  ShieldCheck,
  Sigma,
  Terminal,
  Zap,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { BundledLanguage } from 'shiki/bundle/web'
import { toast } from 'sonner'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import {
  StaticDataTable,
  staticDataTableClassNames as tableStyles,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useStatus } from '@/hooks/use-status'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import {
  AUDIO_SPEECH_ENDPOINT_TYPE,
  buildIndexTTSAudioSpeechAiIntegrationGuide,
  buildIndexTTSAudioSpeechParameters,
  buildIndexTTSAudioSpeechSample,
  supportsAudioSpeechEndpoint,
} from '../lib/audio-speech-api-docs'
import {
  buildDepthMediaAiIntegrationGuide,
  buildDepthMediaJobSample,
} from '../lib/depth-media-catalog'
import {
  buildAsyncImageSample,
  buildImageAiIntegrationGuide,
  IMAGE_SAMPLE_LANGUAGES,
  STANDARD_SAMPLE_LANGUAGES,
  type ImageSampleLanguage,
  withGPTImage2DocumentedParameters,
} from '../lib/image-api-docs'
import {
  buildMiniMaxVideoAiIntegrationGuide,
  buildMiniMaxVideoParameters,
  buildMiniMaxVideoSample,
  MINIMAX_VIDEO_V2_ENDPOINT_TYPE,
  supportsMiniMaxVideoV2Endpoint,
} from '../lib/minimax-video-api-docs'
import {
  buildRateLimits,
  buildSupportedParameters,
  formatRateLimit,
  type SupportedParameter,
} from '../lib/mock-stats'
import { replaceModelInPath } from '../lib/model-helpers'
import type {
  ApiProfileParameter,
  ModelApiProfile,
  PricingModel,
} from '../types'

// ---------------------------------------------------------------------------
// Code-sample registry
// ---------------------------------------------------------------------------
//
// Each sample is keyed by language and endpoint type. The endpoint type comes
// from the model's `supported_endpoint_types`; we render samples only for the
// types the model actually supports. This keeps copy-pasted code accurate and
// provider-shaped (OpenAI vs Anthropic vs Gemini, etc.).

type Lang = ImageSampleLanguage

const LANG_LABELS: Record<Lang, string> = {
  curl: 'cURL',
  bash: 'Bash',
  python: 'Python',
  typescript: 'TypeScript',
  javascript: 'JavaScript',
}

const LANG_HIGHLIGHT: Record<Lang, BundledLanguage> = {
  curl: 'bash',
  bash: 'bash',
  python: 'python',
  typescript: 'typescript',
  javascript: 'javascript',
}

const IMAGE_RUNTIME_HINT_KEYS: Record<Lang, string> = {
  curl: 'cURL example: replace the placeholders before running.',
  bash: 'Bash runtime: Bash, curl, and Python 3.',
  python: 'Python runtime: Python 3.9+ with requests 2.x.',
  typescript:
    'TypeScript runtime: Bun 1.0+, or Node.js 18+ in ESM mode with a TypeScript runner.',
  javascript: 'JavaScript runtime: Bun 1.0+ or Node.js 18+ in ESM mode.',
}

const AUDIO_SPEECH_CURL_RUNTIME_HINT_KEY =
  'cURL writes the submit body to speech.response; a 200 response is WAV, while a 202 response must be recovered from Location.'

const MINIMAX_VIDEO_RUNTIME_HINT_KEYS: Record<Lang, string> = {
  curl: 'cURL runtime: Bash, curl, and Python 3.',
  bash: 'Bash runtime: Bash, curl, and Python 3.',
  python: 'Python runtime: Python 3.9+ with requests 2.x.',
  typescript:
    'TypeScript runtime: Bun 1.0+, or Node.js 18+ in ESM mode with a TypeScript runner.',
  javascript: 'JavaScript runtime: Bun 1.0+ or Node.js 18+ in ESM mode.',
}

type SampleContext = {
  baseUrl: string
  apiKeyEnv: string
  modelName: string
  endpointType: string
  endpointPath: string
  apiProfile?: ModelApiProfile
}

function buildChatSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const isResponses = ctx.endpointType === 'openai-response'
  const isReasoning = /^o[1-4]|reasoning|thinking|deepseek-r/i.test(
    ctx.modelName
  )
  const userMessage = 'Explain quantum entanglement in one paragraph.'

  const bodyJson = isResponses
    ? JSON.stringify({ model: ctx.modelName, input: userMessage }, null, 2)
    : JSON.stringify(
        {
          model: ctx.modelName,
          messages: [{ role: 'user', content: userMessage }],
          ...(isReasoning ? {} : { temperature: 0.7 }),
        },
        null,
        2
      )

  const fnCall = isResponses ? 'responses.create' : 'chat.completions.create'

  if (lang === 'curl') {
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${bodyJson.replaceAll('\n', '\n     ')}'`,
    ].join('\n')
  }

  if (lang === 'python') {
    return [
      'from openai import OpenAI',
      '',
      'client = OpenAI(',
      `    base_url="${ctx.baseUrl}/v1",`,
      `    api_key="<YOUR_API_KEY>",`,
      ')',
      '',
      isResponses
        ? `response = client.${fnCall}(\n    model="${ctx.modelName}",\n    input="${userMessage}",\n)\n\nprint(response.output_text)`
        : `completion = client.${fnCall}(\n    model="${ctx.modelName}",\n    messages=[\n        {"role": "user", "content": "${userMessage}"}\n    ],\n)\n\nprint(completion.choices[0].message.content)`,
    ].join('\n')
  }

  if (lang === 'typescript') {
    return [
      `import OpenAI from 'openai'`,
      '',
      `const client = new OpenAI({`,
      `  baseURL: '${ctx.baseUrl}/v1',`,
      `  apiKey: process.env.${ctx.apiKeyEnv},`,
      `})`,
      '',
      isResponses
        ? `const response = await client.${fnCall}({\n  model: '${ctx.modelName}',\n  input: '${userMessage}',\n})\n\nconsole.log(response.output_text)`
        : `const completion = await client.${fnCall}({\n  model: '${ctx.modelName}',\n  messages: [{ role: 'user', content: '${userMessage}' }],\n})\n\nconsole.log(completion.choices[0].message.content)`,
    ].join('\n')
  }

  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\`,`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify(${bodyJson}),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data)`,
  ].join('\n')
}

function buildAnthropicSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const userMessage = 'Explain quantum entanglement in one paragraph.'

  if (lang === 'curl') {
    const body = JSON.stringify(
      {
        model: ctx.modelName,
        max_tokens: 1024,
        messages: [{ role: 'user', content: userMessage }],
      },
      null,
      2
    )
    return [
      `curl ${url} \\`,
      `  -H "x-api-key: $${ctx.apiKeyEnv}" \\`,
      `  -H "anthropic-version: 2023-06-01" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body.replaceAll('\n', '\n     ')}'`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import anthropic',
      '',
      'client = anthropic.Anthropic(',
      `    base_url="${ctx.baseUrl}",`,
      `    api_key="<YOUR_API_KEY>",`,
      ')',
      '',
      `message = client.messages.create(`,
      `    model="${ctx.modelName}",`,
      `    max_tokens=1024,`,
      `    messages=[{"role": "user", "content": "${userMessage}"}],`,
      ')',
      '',
      'print(message.content[0].text)',
    ].join('\n')
  }
  if (lang === 'typescript') {
    return [
      `import Anthropic from '@anthropic-ai/sdk'`,
      '',
      `const client = new Anthropic({`,
      `  baseURL: '${ctx.baseUrl}',`,
      `  apiKey: process.env.${ctx.apiKeyEnv},`,
      `})`,
      '',
      `const message = await client.messages.create({`,
      `  model: '${ctx.modelName}',`,
      `  max_tokens: 1024,`,
      `  messages: [{ role: 'user', content: '${userMessage}' }],`,
      `})`,
      '',
      `console.log(message.content[0].text)`,
    ].join('\n')
  }
  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    'x-api-key': process.env.${ctx.apiKeyEnv},`,
    `    'anthropic-version': '2023-06-01',`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify({`,
    `    model: '${ctx.modelName}',`,
    `    max_tokens: 1024,`,
    `    messages: [{ role: 'user', content: '${userMessage}' }],`,
    `  }),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data.content[0].text)`,
  ].join('\n')
}

function buildGeminiSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}?key=$${ctx.apiKeyEnv}`
  const userMessage = 'Explain quantum entanglement in one paragraph.'

  if (lang === 'curl') {
    const body = JSON.stringify(
      { contents: [{ parts: [{ text: userMessage }] }] },
      null,
      2
    )
    return [
      `curl '${url}' \\`,
      `  -H 'Content-Type: application/json' \\`,
      `  -d '${body.replaceAll('\n', '\n     ')}'`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import google.generativeai as genai',
      '',
      `genai.configure(api_key="<YOUR_API_KEY>")`,
      '',
      `model = genai.GenerativeModel("${ctx.modelName}")`,
      `response = model.generate_content("${userMessage}")`,
      '',
      `print(response.text)`,
    ].join('\n')
  }
  if (lang === 'typescript') {
    return [
      `import { GoogleGenerativeAI } from '@google/generative-ai'`,
      '',
      `const genAI = new GoogleGenerativeAI(process.env.${ctx.apiKeyEnv}!)`,
      `const model = genAI.getGenerativeModel({ model: '${ctx.modelName}' })`,
      '',
      `const result = await model.generateContent('${userMessage}')`,
      `console.log(result.response.text())`,
    ].join('\n')
  }
  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: { 'Content-Type': 'application/json' },`,
    `  body: JSON.stringify({`,
    `    contents: [{ parts: [{ text: '${userMessage}' }] }],`,
    `  }),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data.candidates[0].content.parts[0].text)`,
  ].join('\n')
}

function buildEmbeddingSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const text = 'The food was delicious and the waiter…'

  if (lang === 'curl') {
    const body = JSON.stringify({ model: ctx.modelName, input: text }, null, 2)
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body.replaceAll('\n', '\n     ')}'`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'from openai import OpenAI',
      '',
      `client = OpenAI(base_url="${ctx.baseUrl}/v1", api_key="<YOUR_API_KEY>")`,
      '',
      'response = client.embeddings.create(',
      `    model="${ctx.modelName}",`,
      `    input="${text}",`,
      ')',
      '',
      'print(response.data[0].embedding[:8])',
    ].join('\n')
  }
  if (lang === 'typescript') {
    return [
      `import OpenAI from 'openai'`,
      '',
      `const client = new OpenAI({`,
      `  baseURL: '${ctx.baseUrl}/v1',`,
      `  apiKey: process.env.${ctx.apiKeyEnv},`,
      `})`,
      '',
      `const response = await client.embeddings.create({`,
      `  model: '${ctx.modelName}',`,
      `  input: '${text}',`,
      `})`,
      '',
      `console.log(response.data[0].embedding.slice(0, 8))`,
    ].join('\n')
  }
  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\`,`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify({`,
    `    model: '${ctx.modelName}',`,
    `    input: '${text}',`,
    `  }),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data.data[0].embedding.slice(0, 8))`,
  ].join('\n')
}

function fallbackImageProfile(endpointPath: string): ModelApiProfile {
  return {
    kind: 'image',
    endpoint: endpointPath,
    async: true,
    poll_endpoint: `${endpointPath}/{task_id}`,
    webhook: true,
    result_delivery: 'oss_url',
    operations: ['generation'],
    parameters: [
      {
        name: 'prompt',
        type: 'string',
        required: true,
        description: 'Text description of the desired image',
      },
      {
        name: 'n',
        type: 'integer',
        default: 1,
        min: 1,
        max: 1,
        description: 'Number of images to generate',
      },
      {
        name: 'response_format',
        type: 'enum',
        default: 'url',
        enum_values: ['url'],
        description: 'How to deliver the resulting image',
      },
      {
        name: 'webhook_url',
        type: 'string',
        description: 'URL receiving image task completion notifications',
      },
      {
        name: 'webhook_secret',
        type: 'string',
        description: 'Secret used to sign webhook deliveries',
      },
    ],
  }
}

function buildSample(
  lang: Lang,
  endpointType: string,
  ctx: SampleContext
): string {
  if (endpointType === 'anthropic') return buildAnthropicSample(lang, ctx)
  if (endpointType === 'gemini') return buildGeminiSample(lang, ctx)
  if (endpointType === 'embeddings' || endpointType === 'jina-rerank') {
    return buildEmbeddingSample(lang, ctx)
  }
  if (endpointType === 'image-generation') {
    return buildAsyncImageSample(lang, {
      baseUrl: ctx.baseUrl,
      apiKeyEnv: ctx.apiKeyEnv,
      modelName: ctx.modelName,
      endpointPath: ctx.endpointPath,
      profile: ctx.apiProfile ?? fallbackImageProfile(ctx.endpointPath),
    })
  }
  if (endpointType === 'depth-media') {
    return buildDepthMediaJobSample(lang, {
      baseUrl: ctx.baseUrl,
      apiKeyEnv: ctx.apiKeyEnv,
      modelName: ctx.modelName,
      endpointPath: ctx.endpointPath,
    })
  }
  if (endpointType === AUDIO_SPEECH_ENDPOINT_TYPE) {
    return buildIndexTTSAudioSpeechSample(lang, {
      baseUrl: ctx.baseUrl,
      apiKeyEnv: ctx.apiKeyEnv,
      modelName: ctx.modelName,
      endpointPath: ctx.endpointPath,
    })
  }
  if (endpointType === MINIMAX_VIDEO_V2_ENDPOINT_TYPE) {
    return buildMiniMaxVideoSample(lang, {
      baseUrl: ctx.baseUrl,
      apiKeyEnv: ctx.apiKeyEnv,
      modelName: ctx.modelName,
      endpointPath: ctx.endpointPath,
    })
  }
  return buildChatSample(lang, ctx)
}

// ---------------------------------------------------------------------------
// Code samples section
// ---------------------------------------------------------------------------

function CodeSamplesSection(props: {
  model: PricingModel
  publicModels: PricingModel[]
  endpoints: Array<{ type: string; path: string; method: string }>
  endpointType: string
  onEndpointTypeChange: (endpointType: string) => void
}) {
  const { t } = useTranslation()
  const { status } = useStatus()

  const baseUrl = useMemo(() => {
    const candidate =
      (status as Record<string, unknown> | null)?.server_address ??
      (status as Record<string, unknown> | null)?.serverAddress ??
      (status?.data as Record<string, unknown> | undefined)?.server_address ??
      (status?.data as Record<string, unknown> | undefined)?.serverAddress
    if (candidate && typeof candidate === 'string') {
      return candidate.replace(/\/$/, '')
    }
    if (typeof window !== 'undefined') return window.location.origin
    return 'https://api.example.com'
  }, [status])

  const [lang, setLang] = useState<Lang>('curl')
  const sampleLanguages =
    props.model.api_profile?.kind === 'image'
      ? IMAGE_SAMPLE_LANGUAGES
      : STANDARD_SAMPLE_LANGUAGES
  const activeLang = sampleLanguages.includes(lang) ? lang : 'curl'

  const activeEndpoint = useMemo(() => {
    return (
      props.endpoints.find((e) => e.type === props.endpointType) ??
      props.endpoints[0]
    )
  }, [props.endpointType, props.endpoints])

  if (props.endpoints.length === 0 || !activeEndpoint) {
    return null
  }

  const isAudioSpeech = activeEndpoint.type === AUDIO_SPEECH_ENDPOINT_TYPE
  const isMiniMaxVideo = activeEndpoint.type === MINIMAX_VIDEO_V2_ENDPOINT_TYPE

  const code = buildSample(activeLang, activeEndpoint.type, {
    baseUrl,
    apiKeyEnv: 'NEW_API_KEY',
    modelName: props.model.model_name || '',
    endpointType: activeEndpoint.type,
    endpointPath: activeEndpoint.path,
    apiProfile: props.model.api_profile,
  })
  let aiIntegrationGuide = ''
  if (props.model.api_profile?.kind === 'image') {
    aiIntegrationGuide = buildImageAiIntegrationGuide({
      baseUrl,
      apiKeyEnv: 'NEW_API_KEY',
      modelName: props.model.model_name || '',
      endpointPath: props.model.api_profile.endpoint,
      profile: props.model.api_profile,
    })
  } else if (props.model.api_profile?.kind === 'media') {
    aiIntegrationGuide = buildDepthMediaAiIntegrationGuide({
      baseUrl,
      apiKeyEnv: 'NEW_API_KEY',
      selectedModel: props.model,
      publicModels: props.publicModels,
    })
  } else if (isAudioSpeech) {
    aiIntegrationGuide = buildIndexTTSAudioSpeechAiIntegrationGuide({
      baseUrl,
      apiKeyEnv: 'NEW_API_KEY',
      modelName: props.model.model_name || '',
      endpointPath: activeEndpoint.path,
    })
  } else if (isMiniMaxVideo) {
    aiIntegrationGuide = buildMiniMaxVideoAiIntegrationGuide({
      baseUrl,
      apiKeyEnv: 'NEW_API_KEY',
      modelName: props.model.model_name || '',
      endpointPath: activeEndpoint.path,
    })
  }

  const copyAiIntegrationGuide = async () => {
    const copied = await copyToClipboard(aiIntegrationGuide)
    if (copied) {
      toast.success(t('Copied to clipboard'))
      return
    }
    toast.error(t('Failed to copy'))
  }

  return (
    <section>
      <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
        <SectionTitle icon={ScrollText}>{t('Code samples')}</SectionTitle>
        {aiIntegrationGuide && (
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8'
            onClick={copyAiIntegrationGuide}
          >
            <Copy aria-hidden='true' className='size-3.5' />
            {t('Copy AI integration guide')}
          </Button>
        )}
      </div>

      {(props.model.api_profile || isMiniMaxVideo) && (
        <div className='mb-3 flex flex-wrap gap-1.5'>
          <Badge variant='secondary'>
            {isMiniMaxVideo ? 200 : 202} {t('Async task')}
          </Badge>
          <Badge variant='outline'>{t('Polling')}</Badge>
          {props.model.api_profile?.webhook && (
            <Badge variant='outline'>Webhook</Badge>
          )}
          {props.model.api_profile?.result_delivery === 'oss_url' && (
            <Badge variant='outline'>{t('OSS URL')}</Badge>
          )}
        </div>
      )}

      <div className='flex flex-wrap items-center gap-2'>
        {props.model.api_profile ? (
          <Badge variant='outline'>
            {props.model.api_profile.kind === 'image'
              ? t('Unified image generation')
              : t('Unified media jobs')}
          </Badge>
        ) : (
          props.endpoints.length > 1 && (
            <Tabs
              value={activeEndpoint.type}
              onValueChange={props.onEndpointTypeChange}
            >
              <TabsList className='bg-muted/40 h-8 p-0.5'>
                {props.endpoints.map((ep) => {
                  let label = ep.type
                  if (ep.type === 'image-generation') {
                    label = t('Unified image generation')
                  } else if (ep.type === AUDIO_SPEECH_ENDPOINT_TYPE) {
                    label = t('Audio')
                  } else if (ep.type === MINIMAX_VIDEO_V2_ENDPOINT_TYPE) {
                    label = t('Video Generation V2')
                  }
                  return (
                    <TabsTrigger
                      key={ep.type}
                      value={ep.type}
                      className='h-7 px-2.5 text-xs'
                    >
                      {label}
                    </TabsTrigger>
                  )
                })}
              </TabsList>
            </Tabs>
          )
        )}

        <Tabs
          value={activeLang}
          onValueChange={(v) => setLang(v as Lang)}
          className='ml-auto'
        >
          <TabsList className='bg-muted/40 h-8 p-0.5'>
            {sampleLanguages.map((l) => (
              <TabsTrigger key={l} value={l} className='h-7 px-2.5 text-xs'>
                {LANG_LABELS[l]}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>

      {props.model.api_profile?.kind === 'image' && (
        <ImageSampleRuntimeHint lang={activeLang} />
      )}
      {isAudioSpeech && <AudioSpeechSampleRuntimeHint lang={activeLang} />}
      {isMiniMaxVideo && <MiniMaxVideoSampleRuntimeHint lang={activeLang} />}

      <div className='mt-3'>
        <CodeBlock code={code} language={LANG_HIGHLIGHT[activeLang]}>
          <CodeBlockCopyButton />
        </CodeBlock>
      </div>

      <p className='text-muted-foreground mt-2 text-xs'>
        <code className='bg-muted rounded px-1 py-0.5 font-mono text-[11px]'>
          NEW_API_KEY
        </code>{' '}
        {t('must contain the API key from your token settings.')}
      </p>

      {isAudioSpeech && <AudioSpeechContractNotice />}
      {isMiniMaxVideo && <MiniMaxVideoContractNotice />}

      {props.model.api_profile && props.model.api_profile.webhook && (
        <WebhookContractNotice profileKind={props.model.api_profile.kind} />
      )}
    </section>
  )
}

function MiniMaxVideoSampleRuntimeHint(props: { lang: Lang }) {
  const { t } = useTranslation()

  return (
    <p className='text-muted-foreground mt-2 flex items-start gap-1.5 text-[11px] leading-relaxed'>
      <Terminal aria-hidden='true' className='mt-0.5 size-3 shrink-0' />
      <span>{t(MINIMAX_VIDEO_RUNTIME_HINT_KEYS[props.lang])}</span>
    </p>
  )
}

function MiniMaxVideoContractNotice() {
  const { t } = useTranslation()

  return (
    <aside className='border-border/60 mt-4 border-t pt-4'>
      <h4 className='text-foreground mb-2 flex items-center gap-1.5 text-xs font-semibold'>
        <ShieldCheck
          aria-hidden='true'
          className='text-muted-foreground/70 size-3.5'
        />
        {t('MiniMax-H3 Video Generation V2 contract')}
      </h4>

      <ul className='text-muted-foreground grid gap-x-6 gap-y-1.5 text-xs leading-relaxed sm:grid-cols-2'>
        <li>
          {t(
            'This endpoint supports the implemented MiniMax V2-compatible subset. Submit returns HTTP 200 with task_id; processing continues in the background, and authenticated querying remains available.'
          )}
        </li>
        <li>
          {t(
            'resolution must be 768P, duration must be an integer from 4 to 15, and ratio must be explicit. 16:9 and 9:16 are always available; 1:1 is unavailable with reference audio.'
          )}
        </li>
        <li>
          {t(
            'content accepts up to 16 items and requires non-empty text. Use up to 9 reference_image items, or up to 3 reference_audio items together with at least one reference image.'
          )}
        </li>
        <li>
          {t(
            'The current implementation does not support 2K, adaptive ratio, first_frame, last_frame, reference_video, or aigc_watermark=true.'
          )}
        </li>
        <li>
          {t(
            'callback_url is optional and must be a public HTTPS URL of at most 2048 characters. It is challenge-verified within 3 seconds before submission and receives only terminal task payloads.'
          )}
        </li>
        <li>
          {t(
            'Terminal callbacks use the query-response body and at-least-once delivery with stable ID and timestamp headers. Return 2xx to acknowledge; there is no signature, and failed delivery does not affect the task.'
          )}
        </li>
        <li>
          {t(
            'Use a unique Idempotency-Key of at most 256 characters. Retry an uncertain submission with the same key and identical JSON, then query the returned task instead of creating a duplicate.'
          )}
        </li>
        <li>
          {t(
            'Tasks are queryable for seven days. Store a successful task.content.url promptly, and use the current model-plaza price because billing is based on the requested duration in seconds.'
          )}
        </li>
      </ul>
    </aside>
  )
}

function AudioSpeechSampleRuntimeHint(props: { lang: Lang }) {
  const { t } = useTranslation()
  const hintKey =
    props.lang === 'curl'
      ? AUDIO_SPEECH_CURL_RUNTIME_HINT_KEY
      : IMAGE_RUNTIME_HINT_KEYS[props.lang]

  return (
    <p className='text-muted-foreground mt-2 flex items-start gap-1.5 text-[11px] leading-relaxed'>
      <Terminal aria-hidden='true' className='mt-0.5 size-3 shrink-0' />
      <span>{t(hintKey)}</span>
    </p>
  )
}

function AudioSpeechContractNotice() {
  const { t } = useTranslation()

  return (
    <aside className='border-border/60 mt-4 border-t pt-4'>
      <h4 className='text-foreground mb-2 flex items-center gap-1.5 text-xs font-semibold'>
        <ShieldCheck
          aria-hidden='true'
          className='text-muted-foreground/70 size-3.5'
        />
        {t('IndexTTS2 speech contract')}
      </h4>

      <ul className='text-muted-foreground grid gap-x-6 gap-y-1.5 text-xs leading-relaxed sm:grid-cols-2'>
        <li>
          {t(
            'Send JSON only and include an Idempotency-Key of at most 256 characters. The same key and canonical request replay the original task without another charge; a different request returns HTTP 409 idempotency_conflict.'
          )}
        </li>
        <li>
          {t(
            'HTTP 200 returns binary audio/wav. HTTP 202 returns task_id and status; follow Location with the same API token after Retry-After until the recovery GET returns WAV.'
          )}
        </li>
        <li>
          {t(
            'voice and metadata.emotion_audio accept a public HTTPS WAV/MP3 URL or a matching base64 data URI. Named OpenAI voices are not supported.'
          )}
        </li>
        <li>
          {t(
            'input accepts 1–2048 UTF-8 characters. The request body is limited to 64 MiB, each reference audio to 15 MiB, both references together to 30 MiB, reference duration to 10 minutes, and generated WAV to 64 MiB.'
          )}
        </li>
        <li>
          {t(
            'metadata.emotion_vector uses exactly eight values in this order: happy, angry, sad, afraid, disgusted, melancholic, surprised, calm. Values must be 0–1.4 and surprised must be 0.'
          )}
        </li>
        <li>
          {t(
            'emotion_audio, emotion_vector, and emotion_random=true are mutually exclusive. instructions and SSE streaming are unsupported; response_format must be wav and speed must be 1.'
          )}
        </li>
      </ul>
    </aside>
  )
}

function ImageSampleRuntimeHint(props: { lang: Lang }) {
  const { t } = useTranslation()

  return (
    <p className='text-muted-foreground mt-2 flex items-start gap-1.5 text-[11px] leading-relaxed'>
      <Terminal aria-hidden='true' className='mt-0.5 size-3 shrink-0' />
      <span>{t(IMAGE_RUNTIME_HINT_KEYS[props.lang])}</span>
    </p>
  )
}

function WebhookContractNotice(props: {
  profileKind: ModelApiProfile['kind']
}) {
  const { t } = useTranslation()

  return (
    <aside className='border-border/60 mt-4 border-t pt-4'>
      <h4 className='text-foreground mb-2 flex items-center gap-1.5 text-xs font-semibold'>
        <ShieldCheck
          aria-hidden='true'
          className='text-muted-foreground/70 size-3.5'
        />
        {t('Webhook delivery and signature')}
      </h4>

      <ul className='text-muted-foreground grid gap-x-6 gap-y-1.5 text-xs leading-relaxed sm:grid-cols-2'>
        <li>
          {props.profileKind === 'media'
            ? t(
                'webhook_url and webhook_secret are optional top-level request fields. The callback must be a publicly reachable HTTPS URL.'
              )
            : t(
                'webhook_url and webhook_secret are optional fields inside input. The callback must be a publicly reachable HTTPS URL.'
              )}
        </li>
        <li>
          {t(
            'Terminal task notifications use at-least-once delivery. Deduplicate retries with X-Webhook-Delivery-Id and return a 2xx response.'
          )}
        </li>
        <li className='sm:col-span-2'>
          {t(
            'When webhook_secret is set, verify X-Webhook-Timestamp, X-Webhook-Delivery-Id, and X-Webhook-Signature, and reject timestamps outside your replay window.'
          )}
        </li>
      </ul>

      <p className='text-muted-foreground mt-2 text-xs leading-relaxed'>
        {t('Signing input')}:{' '}
        <code className='bg-muted rounded px-1 py-0.5 font-mono text-[11px]'>
          v1.timestamp.deliveryID.rawBody
        </code>
        .{' '}
        {t(
          'Compute HMAC-SHA256 with webhook_secret and compare X-Webhook-Signature (v1=<hex>) in constant time.'
        )}
      </p>
    </aside>
  )
}

// ---------------------------------------------------------------------------
// Supported parameters table
// ---------------------------------------------------------------------------

function SupportedParametersSection(props: {
  model: PricingModel
  endpointType: string
}) {
  const { t } = useTranslation()
  const displayProfile = useMemo(() => {
    if (!props.model.api_profile) return undefined
    return withGPTImage2DocumentedParameters(
      props.model.model_name || '',
      props.model.api_profile
    )
  }, [props.model.api_profile, props.model.model_name])
  const params = useMemo(() => {
    if (props.endpointType === MINIMAX_VIDEO_V2_ENDPOINT_TYPE) {
      return buildMiniMaxVideoParameters(props.model.model_name || 'MiniMax-H3')
    }
    if (props.endpointType === AUDIO_SPEECH_ENDPOINT_TYPE) {
      return buildIndexTTSAudioSpeechParameters(
        props.model.model_name || 'indextts2-v1'
      )
    }
    if (displayProfile) {
      return displayProfile.parameters.map((parameter) =>
        profileParameterForDisplay(parameter, displayProfile.kind)
      )
    }
    return buildSupportedParameters(props.model)
  }, [displayProfile, props.endpointType, props.model])

  if (params.length === 0) return null

  return (
    <section>
      <SectionTitle icon={Sigma}>{t('Supported parameters')}</SectionTitle>
      <StaticDataTable
        className={tableStyles.sectionContainer}
        headerRowClassName={tableStyles.mutedHeaderRow}
        data={params}
        getRowKey={(param) => param.name}
        getRowClassName={() => 'hover:bg-muted/20'}
        columns={[
          {
            id: 'parameter',
            header: t('Parameter'),
            className: 'h-9 w-44',
            cellClassName: tableStyles.topCell,
            cell: (p) => (
              <div className='flex items-center gap-1.5'>
                <code className='font-mono text-sm font-medium'>{p.name}</code>
                {p.required && (
                  <Badge
                    variant='outline'
                    className='h-6 border-rose-500/40 px-2 text-sm text-rose-600 dark:text-rose-400'
                  >
                    {t('required')}
                  </Badge>
                )}
              </div>
            ),
          },
          {
            id: 'type',
            header: t('Type'),
            className: 'h-9 w-24',
            cellClassName: tableStyles.topCell,
            cell: (p) => (
              <Badge
                variant='secondary'
                className='h-7 rounded-full px-2.5 font-mono text-sm font-normal'
              >
                {p.type}
              </Badge>
            ),
          },
          {
            id: 'range',
            header: t('Default / allowed values'),
            className: 'h-9 w-72',
            cellClassName: tableStyles.topCell,
            cell: (p) => <ParamRangeCell param={p} />,
          },
          {
            id: 'description',
            header: t('Description'),
            className: 'h-9',
            cellClassName: tableStyles.topMutedCell,
            cell: (p) => t(p.descriptionKey),
          },
        ]}
      />
      <ApiProfileConstraints profile={displayProfile} />
    </section>
  )
}

function profileParameterForDisplay(
  parameter: ApiProfileParameter,
  profileKind?: ModelApiProfile['kind']
): SupportedParameter {
  let range: string | undefined
  if (parameter.min !== undefined && parameter.max !== undefined) {
    range = `${parameter.min} ~ ${parameter.max}`
  } else if (parameter.min !== undefined) {
    range = `>= ${parameter.min}`
  } else if (parameter.max !== undefined) {
    range = `<= ${parameter.max}`
  } else if (parameter.max_items !== undefined) {
    range = `1 ~ ${parameter.max_items}`
  }

  return {
    name: parameter.name,
    type: parameter.type,
    required: parameter.required,
    defaultValue: parameter.default,
    enumValues: parameter.enum_values,
    range,
    descriptionKey:
      (profileKind === 'media'
        ? MEDIA_PARAMETER_DESCRIPTION_KEYS[parameter.name]
        : undefined) ||
      IMAGE_PARAMETER_DESCRIPTION_KEYS[parameter.name] ||
      parameter.description ||
      parameter.name,
  }
}

const MEDIA_PARAMETER_DESCRIPTION_KEYS: Record<string, string> = {
  format: 'Output media format',
  quality: 'Media processing quality profile',
  subtitle_area: 'Area to scan for hard-coded subtitles',
  webhook_url: 'URL receiving asynchronous task completion notifications',
  webhook_secret: 'Secret used to sign asynchronous task webhook deliveries',
}

const IMAGE_PARAMETER_DESCRIPTION_KEYS: Record<string, string> = {
  source_url: 'Publicly reachable source image or video URL',
  operation: 'Media processing operation',
  prompt: 'Text description of the desired image',
  image_input: 'Reference image URLs for image editing',
  aspect_ratio: 'Output aspect ratio supported by the selected model',
  resolution: 'Output resolution supported by the selected model',
  size: 'Output image size',
  quality: 'Generation quality preset',
  scale: 'Upscale multiplier',
  format: 'Generated image file format',
  n: 'Number of images to generate',
  output_format: 'Generated image file format',
  output_compression: 'Output compression level from 0 to 100',
  background: 'Background treatment for the generated image',
  moderation: 'Safety moderation level for image generation',
  watermark: 'Whether to add a provider watermark',
  prompt_optimizer: 'Whether to optimize the prompt before generation',
  parameters: 'Provider-specific image parameters',
  extra_fields: 'Provider-specific image parameters',
  negative_prompt: 'Content to exclude from the generated image',
  batch_size: 'Number of images requested from the provider',
  seed: 'Random seed used by the image model',
  num_inference_steps: 'Number of denoising steps',
  guidance_scale: 'Guidance scale used by the image model',
  cfg: 'Classifier-free guidance value',
  response_format: 'How to deliver the resulting image',
  webhook_url: 'URL receiving image task completion notifications',
  webhook_secret: 'Secret used to sign webhook deliveries',
}

function ParamRangeCell(props: { param: SupportedParameter }) {
  const { t } = useTranslation()
  const { defaultValue, range, enumValues } = props.param
  if (
    defaultValue === undefined &&
    !range &&
    (!enumValues || enumValues.length === 0)
  ) {
    return <span className='text-muted-foreground/60 text-sm'>—</span>
  }

  return (
    <div className='space-y-1.5'>
      {defaultValue !== undefined && (
        <div className='flex flex-wrap items-center gap-1'>
          <span className='text-muted-foreground text-xs'>{t('Default')}:</span>
          <code className='bg-muted rounded px-1.5 py-0.5 font-mono text-sm'>
            {String(defaultValue)}
          </code>
        </div>
      )}
      {enumValues && enumValues.length > 0 && (
        <div className='flex flex-wrap gap-1'>
          {enumValues.map((v) => (
            <code
              key={v}
              className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 font-mono text-xs'
            >
              {v}
            </code>
          ))}
        </div>
      )}
      {range && (
        <span className='text-muted-foreground block font-mono text-xs'>
          {range}
        </span>
      )}
    </div>
  )
}

function ApiProfileConstraints(props: { profile?: ModelApiProfile }) {
  const { t } = useTranslation()
  const constraints = props.profile?.constraints || []
  const combinations = constraints.flatMap((constraint) =>
    constraint.type === 'allowed_combinations' ? constraint.combinations : []
  )

  if (combinations.length === 0) return null

  return (
    <div className='mt-3'>
      <p className='text-muted-foreground mb-2 text-xs font-medium'>
        {t('Supported combinations')}
      </p>
      <div className='flex flex-wrap gap-1.5'>
        {combinations.map((combination) => {
          const label = Object.entries(combination)
            .map(([field, value]) => `${field}=${value}`)
            .join(' · ')
          return (
            <code
              key={label}
              className='bg-muted text-muted-foreground rounded px-2 py-1 font-mono text-xs'
            >
              {label}
            </code>
          )
        })}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Rate-limits table
// ---------------------------------------------------------------------------

function RateLimitsSection(props: { model: PricingModel }) {
  const { t } = useTranslation()
  const limits = useMemo(() => buildRateLimits(props.model), [props.model])

  if (limits.length === 0) return null

  return (
    <section>
      <SectionTitle icon={Gauge}>{t('Rate limits')}</SectionTitle>
      <StaticDataTable
        className={tableStyles.sectionContainer}
        headerRowClassName={tableStyles.mutedHeaderRow}
        data={limits}
        getRowKey={(limit) => limit.group}
        getRowClassName={() => 'hover:bg-muted/20'}
        columns={[
          {
            id: 'group',
            header: t('Group'),
            className: 'h-9',
            cellClassName: 'py-2 font-mono',
            cell: (limit) => limit.group,
          },
          {
            id: 'rpm',
            header: 'RPM',
            className: 'h-9 text-right',
            cellClassName: tableStyles.topNumericCell,
            cell: (limit) => formatRateLimit(limit.rpm),
          },
          {
            id: 'tpm',
            header: 'TPM',
            className: 'h-9 text-right',
            cellClassName: tableStyles.topNumericCell,
            cell: (limit) => formatRateLimit(limit.tpm),
          },
          {
            id: 'rpd',
            header: 'RPD',
            className: 'h-9 text-right',
            cellClassName: tableStyles.topNumericCell,
            cell: (limit) => formatRateLimit(limit.rpd),
          },
        ]}
      />
      <p className='text-muted-foreground mt-2 text-[11px] leading-relaxed'>
        {t(
          'RPM = requests per minute, TPM = tokens per minute, RPD = requests per day. Limits apply per token group.'
        )}
      </p>
    </section>
  )
}

// ---------------------------------------------------------------------------
// Authentication preview
// ---------------------------------------------------------------------------

function AuthSection() {
  const { t } = useTranslation()
  return (
    <section>
      <SectionTitle icon={KeyRound}>{t('Authentication')}</SectionTitle>
      <div className='border-border/60 bg-muted/20 flex items-start gap-2 rounded-lg border p-3'>
        <ChevronRight className='text-muted-foreground mt-0.5 size-3.5 shrink-0' />
        <div className='space-y-1.5 text-xs leading-relaxed'>
          <p>
            {t('All requests must include')}{' '}
            <code className='bg-muted rounded px-1 py-0.5 font-mono text-[11px]'>
              Authorization: Bearer &lt;TOKEN&gt;
            </code>{' '}
            {t('header. Anthropic-formatted endpoints accept the')}{' '}
            <code className='bg-muted rounded px-1 py-0.5 font-mono text-[11px]'>
              x-api-key
            </code>{' '}
            {t('header instead.')}
          </p>
          <p className='text-muted-foreground'>
            {t(
              'Generate tokens from the Tokens page; you can scope them to specific models, groups, IPs, and rate-limits.'
            )}
          </p>
        </div>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// Composite API tab
// ---------------------------------------------------------------------------

export function ModelDetailsApi(props: {
  model: PricingModel
  publicModels?: PricingModel[]
  endpointMap: Record<string, { path?: string; method?: string }>
}) {
  const publicModels = props.publicModels?.length
    ? props.publicModels
    : [props.model]
  const endpoints = useMemo(() => {
    if (props.model.api_profile) {
      return [
        {
          type:
            props.model.api_profile.kind === 'image'
              ? 'image-generation'
              : 'depth-media',
          path: props.model.api_profile.endpoint,
          method: 'POST',
        },
      ]
    }

    const types = props.model.supported_endpoint_types || []
    return types
      .map((type) => {
        const info = props.endpointMap[type] || {}
        let path = info.path || ''
        if (path && path.includes('{model}')) {
          path = replaceModelInPath(path, props.model.model_name || '')
        }
        return { type, path, method: info.method || 'POST' }
      })
      .filter((endpoint) => Boolean(endpoint.path))
  }, [props.endpointMap, props.model])
  const [selectedEndpointType, setSelectedEndpointType] = useState('')
  const activeEndpointType =
    endpoints.find((endpoint) => endpoint.type === selectedEndpointType)
      ?.type ??
    endpoints[0]?.type ??
    ''

  return (
    <div className='space-y-6'>
      <CodeSamplesSection
        model={props.model}
        publicModels={publicModels}
        endpoints={endpoints}
        endpointType={activeEndpointType}
        onEndpointTypeChange={setSelectedEndpointType}
      />
      <AuthSection />
      <SupportedParametersSection
        model={props.model}
        endpointType={activeEndpointType}
      />
      {!props.model.api_profile &&
        !supportsAudioSpeechEndpoint(props.model) &&
        !supportsMiniMaxVideoV2Endpoint(props.model) && (
          <RateLimitsSection model={props.model} />
        )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Local UI helpers
// ---------------------------------------------------------------------------

function SectionTitle(props: {
  children: React.ReactNode
  icon: React.ComponentType<{ className?: string }>
}) {
  const Icon = props.icon
  return (
    <h3 className='text-foreground mb-3 flex items-center gap-1.5 text-sm font-semibold'>
      <Icon className='text-muted-foreground/70 size-3.5' />
      {props.children}
    </h3>
  )
}

// Re-export so the parent can keep its own SectionTitle if it wants:
export { Zap as ApiTabIcon }
