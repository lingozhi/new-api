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
import type {
  ModelApiProfile,
  ModelPricingVariant,
  PricingModel,
} from '../types'

export type DepthMediaSampleLanguage =
  | 'curl'
  | 'bash'
  | 'python'
  | 'typescript'
  | 'javascript'

export type DepthMediaSampleContext = {
  baseUrl: string
  apiKeyEnv: string
  modelName: string
  endpointPath: string
}

const DEPTH_MODEL = 'depth-anything-v2-small-video'

const BACKGROUND_PROFILES = [
  {
    source: 'background-remove-fast',
    label: 'Fast background removal',
    quality: 'fast',
  },
  {
    source: 'background-remove-quality',
    label: 'High-quality background removal',
    quality: 'quality',
  },
  {
    source: 'background-remove-matting',
    label: 'Precision matting',
    quality: 'matting',
  },
] as const

const UPSCALE_PROFILES = [
  {
    source: 'image-upscale-fast-2x',
    label: 'Fast 2x',
    quality: 'fast',
    scale: 2,
  },
  {
    source: 'image-upscale-fast-4x',
    label: 'Fast 4x',
    quality: 'fast',
    scale: 4,
  },
  {
    source: 'image-upscale-fidelity-4x',
    label: 'Fidelity 4x',
    quality: 'fidelity',
    scale: 4,
  },
  {
    source: 'image-upscale-sharp-4x',
    label: 'Sharp 4x',
    quality: 'sharp',
    scale: 4,
  },
] as const

const SOURCE_MODEL_NAMES = new Set([
  DEPTH_MODEL,
  ...BACKGROUND_PROFILES.map((profile) => profile.source),
  ...UPSCALE_PROFILES.map((profile) => profile.source),
])

function mediaProfile(
  operation: string,
  parameters: ModelApiProfile['parameters'],
  pricingVariants: ModelPricingVariant[]
): ModelApiProfile {
  return {
    kind: 'media',
    endpoint: '/v1/jobs',
    async: true,
    poll_endpoint: '/v1/jobs/{task_id}',
    webhook: true,
    result_delivery: 'oss_url',
    operations: [operation],
    parameters: [
      {
        name: 'source_url',
        type: 'string',
        required: true,
        description: 'Publicly reachable source image or video URL',
      },
      {
        name: 'operation',
        type: 'enum',
        required: true,
        default: operation,
        enum_values: [operation],
        description: 'Media processing operation',
      },
      ...parameters,
      {
        name: 'webhook_url',
        type: 'string',
        description: 'Optional HTTPS callback URL',
      },
      {
        name: 'webhook_secret',
        type: 'string',
        description: 'Optional webhook signature secret',
      },
    ],
    pricing_variants: pricingVariants,
  }
}

function mergeGroups(models: PricingModel[]): string[] {
  return [...new Set(models.flatMap((model) => model.enable_groups || []))]
}

function minimumPrice(models: PricingModel[]): number {
  return Math.min(
    ...models.map((model) => model.model_price ?? Number.POSITIVE_INFINITY)
  )
}

function consolidatedModel(
  sourceModels: PricingModel[],
  modelName: string,
  description: string,
  tags: string,
  apiProfile: ModelApiProfile
): PricingModel {
  const source = sourceModels[0]
  return {
    ...source,
    model_name: modelName,
    key: modelName,
    description,
    tags,
    quota_type: 1,
    model_price: minimumPrice(sourceModels),
    enable_groups: mergeGroups(sourceModels),
    supported_endpoint_types: ['depth-media'],
    api_profile: apiProfile,
  }
}

export function consolidateDepthMediaModels(
  models: PricingModel[],
  translate: (key: string) => string = (key) => key
): PricingModel[] {
  const indexed = new Map(models.map((model) => [model.model_name, model]))
  const depth = indexed.get(DEPTH_MODEL)
  const background = BACKGROUND_PROFILES.flatMap((profile) => {
    const model = indexed.get(profile.source)
    return model ? [{ model, profile }] : []
  })
  const upscale = UPSCALE_PROFILES.flatMap((profile) => {
    const model = indexed.get(profile.source)
    return model ? [{ model, profile }] : []
  })

  const publicModels: PricingModel[] = []
  if (depth) {
    publicModels.push(
      consolidatedModel(
        [depth],
        'depth-video',
        translate(
          'Convert video into a grayscale depth map, up to 10 minutes.'
        ),
        translate('Video,Depth map'),
        mediaProfile(
          'depth',
          [],
          [
            {
              label: translate('Depth video'),
              parameters: { operation: 'depth' },
              price: depth.model_price ?? 0,
              unit: 'second',
            },
          ]
        )
      )
    )
  }
  if (background.length > 0) {
    publicModels.push(
      consolidatedModel(
        background.map((entry) => entry.model),
        'background-remove',
        translate(
          'Remove image backgrounds with fast, high-quality, or precision matting.'
        ),
        translate('Image,Background removal,Matting'),
        mediaProfile(
          'remove_background',
          [
            {
              name: 'quality',
              type: 'enum',
              required: true,
              default: 'fast',
              enum_values: background.map((entry) => entry.profile.quality),
              description: 'Processing quality profile',
            },
            {
              name: 'format',
              type: 'enum',
              default: 'webp',
              enum_values: ['png', 'webp'],
              description: 'Output image format',
            },
          ],
          background.map((entry) => ({
            label: translate(entry.profile.label),
            parameters: {
              operation: 'remove_background',
              quality: entry.profile.quality,
            },
            price: entry.model.model_price ?? 0,
            unit: 'request',
          }))
        )
      )
    )
  }
  if (upscale.length > 0) {
    publicModels.push(
      consolidatedModel(
        upscale.map((entry) => entry.model),
        'image-upscale',
        translate('Upscale images with fast, fidelity, or sharp profiles.'),
        translate('Image,Upscale'),
        mediaProfile(
          'upscale',
          [
            {
              name: 'quality',
              type: 'enum',
              required: true,
              default: 'fast',
              enum_values: [
                ...new Set(upscale.map((entry) => entry.profile.quality)),
              ],
              description: 'Upscale quality profile',
            },
            {
              name: 'scale',
              type: 'enum',
              required: true,
              default: 2,
              enum_values: [
                ...new Set(upscale.map((entry) => String(entry.profile.scale))),
              ],
              description: 'Upscale multiplier',
            },
            {
              name: 'format',
              type: 'enum',
              default: 'webp',
              enum_values: ['png', 'webp'],
              description: 'Output image format',
            },
          ],
          upscale.map((entry) => ({
            label: translate(entry.profile.label),
            parameters: {
              operation: 'upscale',
              quality: entry.profile.quality,
              scale: entry.profile.scale,
            },
            price: entry.model.model_price ?? 0,
            unit: 'request',
          }))
        )
      )
    )
  }

  if (publicModels.length === 0) return models

  const firstSourceIndex = models.findIndex((model) =>
    SOURCE_MODEL_NAMES.has(model.model_name)
  )
  const remaining = models.filter(
    (model) => !SOURCE_MODEL_NAMES.has(model.model_name)
  )
  const insertionIndex = Math.max(firstSourceIndex, 0)
  return [
    ...remaining.slice(0, insertionIndex),
    ...publicModels,
    ...remaining.slice(insertionIndex),
  ]
}

function samplePayload(modelName: string): Record<string, string | number> {
  switch (modelName) {
    case 'depth-video':
      return {
        model: modelName,
        source_url: 'https://cdn.example.com/input.mp4',
        operation: 'depth',
      }
    case 'background-remove':
      return {
        model: modelName,
        source_url: 'https://cdn.example.com/input.png',
        operation: 'remove_background',
        quality: 'fast',
        format: 'webp',
      }
    default:
      return {
        model: modelName,
        source_url: 'https://cdn.example.com/input.png',
        operation: 'upscale',
        quality: 'fast',
        scale: 2,
        format: 'webp',
      }
  }
}

export function buildDepthMediaJobSample(
  language: DepthMediaSampleLanguage,
  context: DepthMediaSampleContext
): string {
  const submitUrl = `${context.baseUrl}${context.endpointPath}`
  const pollUrl = `${submitUrl}/<TASK_ID>`
  const payload = samplePayload(context.modelName)
  const payloadJson = JSON.stringify(payload, null, 2)

  if (language === 'curl' || language === 'bash') {
    return [
      `curl -X POST ${submitUrl} \\`,
      `  -H "Authorization: Bearer $${context.apiKeyEnv}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${payloadJson.replaceAll('\n', '\n     ')}'`,
      '',
      '# Use task_id from the 202 response to query the result',
      `curl ${pollUrl} \\`,
      `  -H "Authorization: Bearer $${context.apiKeyEnv}"`,
    ].join('\n')
  }

  if (language === 'python') {
    return [
      'import os',
      'import time',
      'import requests',
      '',
      `headers = {"Authorization": f"Bearer {os.environ['${context.apiKeyEnv}']}"}`,
      `response = requests.post("${submitUrl}", headers=headers, json=${JSON.stringify(payload)})`,
      'response.raise_for_status()',
      'task = response.json()',
      '',
      'while True:',
      `    result_response = requests.get(f"${submitUrl}/{task['task_id']}", headers=headers)`,
      '    result_response.raise_for_status()',
      '    result = result_response.json()',
      '    status = result.get("data", result).get("status")',
      '    if status in {"SUCCESS", "FAILURE"}:',
      '        break',
      '    time.sleep(3)',
      '',
      'print(result)',
    ].join('\n')
  }

  const declaration =
    language === 'typescript' ? 'const task: { task_id: string }' : 'const task'
  return [
    `const headers = {`,
    `  Authorization: \`Bearer \${process.env.${context.apiKeyEnv}}\`,`,
    `  'Content-Type': 'application/json',`,
    `}`,
    '',
    `const response = await fetch('${submitUrl}', {`,
    `  method: 'POST',`,
    `  headers,`,
    `  body: JSON.stringify(${payloadJson}),`,
    `})`,
    `if (!response.ok) throw new Error(\`Submit failed: \${response.status}\`)`,
    `${declaration} = await response.json()`,
    '',
    `let result`,
    `while (true) {`,
    `  const pollResponse = await fetch(\`${submitUrl}/\${task.task_id}\`, { headers })`,
    `  if (!pollResponse.ok) throw new Error(\`Poll failed: \${pollResponse.status}\`)`,
    `  result = await pollResponse.json()`,
    `  const status = result.data?.status ?? result.status`,
    `  if (status === 'SUCCESS' || status === 'FAILURE') break`,
    `  await new Promise((resolve) => setTimeout(resolve, 3000))`,
    `}`,
    '',
    `console.log(result)`,
  ].join('\n')
}
