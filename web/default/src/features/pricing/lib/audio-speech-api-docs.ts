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
import type { PricingModel } from '../types'
import type { ImageSampleLanguage } from './image-api-docs'
import type { SupportedParameter } from './mock-stats'

export const AUDIO_SPEECH_ENDPOINT_TYPE = 'audio-speech'

export type AudioSpeechSampleContext = {
  baseUrl: string
  apiKeyEnv: string
  modelName: string
  endpointPath: string
}

const REFERENCE_AUDIO_PLACEHOLDER = '<REFERENCE_AUDIO_URL>'
const EMOTION_AUDIO_PLACEHOLDER = '<EMOTION_AUDIO_URL>'

export function supportsAudioSpeechEndpoint(model: PricingModel): boolean {
  return (
    model.supported_endpoint_types?.includes(AUDIO_SPEECH_ENDPOINT_TYPE) ??
    false
  )
}

export function buildIndexTTSAudioSpeechParameters(
  modelName: string
): SupportedParameter[] {
  return [
    {
      name: 'model',
      type: 'string',
      required: true,
      defaultValue: modelName,
      descriptionKey: 'Exact model identifier for IndexTTS2 speech synthesis',
    },
    {
      name: 'emo_afraid',
      type: 'number',
      defaultValue: 0,
      range: '0 ~ 1.4',
      descriptionKey: 'Emotion intensity from 0 to 1.4; defaults to 0',
    },
    {
      name: 'emo_angry',
      type: 'number',
      defaultValue: 0,
      range: '0 ~ 1.4',
      descriptionKey: 'Emotion intensity from 0 to 1.4; defaults to 0',
    },
    {
      name: 'emo_calm',
      type: 'number',
      defaultValue: 0,
      range: '0 ~ 1.4',
      descriptionKey: 'Emotion intensity from 0 to 1.4; defaults to 0',
    },
    {
      name: 'emo_control_method',
      type: 'enum',
      required: true,
      defaultValue: '与音色参考音频相同',
      enumValues: ['与音色参考音频相同'],
      descriptionKey:
        'Emotion control mode; currently only the speaker reference is supported',
    },
    {
      name: 'emo_disgusted',
      type: 'number',
      defaultValue: 0,
      range: '0 ~ 1.4',
      descriptionKey: 'Emotion intensity from 0 to 1.4; defaults to 0',
    },
    {
      name: 'emo_happy',
      type: 'number',
      defaultValue: 0,
      range: '0 ~ 1.4',
      descriptionKey: 'Emotion intensity from 0 to 1.4; defaults to 0',
    },
    {
      name: 'emo_melancholic',
      type: 'number',
      defaultValue: 0,
      range: '0 ~ 1.4',
      descriptionKey: 'Emotion intensity from 0 to 1.4; defaults to 0',
    },
    {
      name: 'emo_random',
      type: 'boolean',
      defaultValue: false,
      enumValues: ['false', 'true'],
      descriptionKey: 'Whether to randomize emotion; defaults to false',
    },
    {
      name: 'emo_ref_audio',
      type: 'string',
      descriptionKey:
        'Optional emotion reference audio as a public HTTPS WAV/MP3 URL or matching base64 data URI',
    },
    {
      name: 'emo_sad',
      type: 'number',
      defaultValue: 0,
      range: '0 ~ 1.4',
      descriptionKey: 'Emotion intensity from 0 to 1.4; defaults to 0',
    },
    {
      name: 'emo_surprised',
      type: 'enum',
      defaultValue: '0',
      enumValues: ['0'],
      descriptionKey: 'Surprise emotion is currently fixed at 0',
    },
    {
      name: 'prompt_simple',
      type: 'string',
      required: true,
      descriptionKey:
        'Reference speaker audio as a public HTTPS WAV/MP3 URL or matching base64 data URI',
    },
    {
      name: 'prompt_text',
      type: 'string',
      required: true,
      range: '1 ~ 2048',
      descriptionKey: 'UTF-8 text to synthesize, from 1 to 2048 characters',
    },
  ]
}

function audioSpeechRequestBody(context: AudioSpeechSampleContext): string {
  return JSON.stringify(
    {
      model: context.modelName,
      emo_sad: 0,
      emo_calm: 0.3,
      emo_angry: 0,
      emo_happy: 0.5,
      emo_afraid: 0,
      emo_random: false,
      prompt_text: '你好，这是一段 IndexTTS2 语音合成示例。',
      emo_disgusted: 0,
      emo_ref_audio: EMOTION_AUDIO_PLACEHOLDER,
      emo_surprised: '0',
      prompt_simple: REFERENCE_AUDIO_PLACEHOLDER,
      emo_melancholic: 0,
      emo_control_method: '与音色参考音频相同',
    },
    null,
    2
  )
}

function buildCurlSample(context: AudioSpeechSampleContext): string {
  const body = audioSpeechRequestBody(context)

  return [
    `# Set ${context.apiKeyEnv}; replace <UNIQUE_ID>, <REFERENCE_AUDIO_URL>, and <EMOTION_AUDIO_URL>.`,
    `BASE_URL="${context.baseUrl}"`,
    `SUBMIT_URL="$BASE_URL${context.endpointPath}"`,
    'HEADERS_FILE="speech.headers"',
    'BODY_FILE="speech.response"',
    '',
    'status=$(curl --silent --show-error --request POST "$SUBMIT_URL" \\',
    `  -H "Authorization: Bearer $${context.apiKeyEnv}" \\`,
    '  -H "Content-Type: application/json" \\',
    '  -H "Idempotency-Key: indextts2-<UNIQUE_ID>" \\',
    '  --dump-header "$HEADERS_FILE" \\',
    '  --output "$BODY_FILE" \\',
    "  --write-out '%{http_code}' \\",
    `  -d '${body.replaceAll('\n', '\n     ')}')`,
    '',
    `location=$(awk 'tolower($1) == "location:" { print $2 }' "$HEADERS_FILE" | tr -d '\\r')`,
    'while [ "$status" = "202" ] || { [ "$status" = "429" ] && [ -n "$location" ]; }; do',
    '  if [ "$status" = "202" ] && [ -z "$location" ]; then',
    '    echo "HTTP 202 response did not include Location" >&2',
    '    exit 1',
    '  fi',
    `  retry_after=$(awk 'tolower($1) == "retry-after:" { print $2 }' "$HEADERS_FILE" | tr -d '\\r')`,
    '  sleep "${retry_after:-2}"',
    '  status=$(curl --silent --show-error --request GET "$BASE_URL$location" \\',
    `    -H "Authorization: Bearer $${context.apiKeyEnv}" \\`,
    '    --dump-header "$HEADERS_FILE" \\',
    '    --output "$BODY_FILE" \\',
    "    --write-out '%{http_code}')",
    `  next_location=$(awk 'tolower($1) == "location:" { print $2 }' "$HEADERS_FILE" | tr -d '\\r')`,
    '  if [ -n "$next_location" ]; then location="$next_location"; fi',
    'done',
    '',
    'if [ "$status" != "200" ]; then',
    '  cat "$BODY_FILE" >&2',
    '  exit 1',
    'fi',
    `content_type=$(awk 'tolower($1) == "content-type:" { print tolower($2) }' "$HEADERS_FILE" | tr -d '\\r')`,
    'if [ "$content_type" != "audio/wav" ]; then',
    '  echo "Expected audio/wav, got ${content_type:-missing Content-Type}" >&2',
    '  exit 1',
    'fi',
    'mv "$BODY_FILE" speech.wav',
    'echo "Saved speech.wav"',
  ].join('\n')
}

function buildPythonSample(context: AudioSpeechSampleContext): string {
  const submitUrl = `${context.baseUrl}${context.endpointPath}`

  return [
    'import os',
    'import time',
    'import uuid',
    'from pathlib import Path',
    'from urllib.parse import urljoin',
    '',
    'import requests',
    '',
    `api_key = os.environ.get("${context.apiKeyEnv}")`,
    'if not api_key:',
    `    raise RuntimeError("Set ${context.apiKeyEnv} before running")`,
    '',
    `submit_url = "${submitUrl}"`,
    'headers = {',
    '    "Authorization": f"Bearer {api_key}",',
    '    "Content-Type": "application/json",',
    '    "Idempotency-Key": f"indextts2-{uuid.uuid4()}",',
    '}',
    'payload = {',
    `    "model": "${context.modelName}",`,
    '    "emo_sad": 0,',
    '    "emo_calm": 0.3,',
    '    "emo_angry": 0,',
    '    "emo_happy": 0.5,',
    '    "emo_afraid": 0,',
    '    "emo_random": False,',
    '    "prompt_text": "你好，这是一段 IndexTTS2 语音合成示例。",',
    '    "emo_disgusted": 0,',
    `    "emo_ref_audio": "${EMOTION_AUDIO_PLACEHOLDER}",  # Replace before running.`,
    '    "emo_surprised": "0",',
    `    "prompt_simple": "${REFERENCE_AUDIO_PLACEHOLDER}",  # Replace before running.`,
    '    "emo_melancholic": 0,',
    '    "emo_control_method": "与音色参考音频相同",',
    '}',
    '',
    'response = requests.post(submit_url, headers=headers, json=payload, timeout=420)',
    'deadline = time.monotonic() + 900',
    'location = response.headers.get("Location")',
    'recovery_url = urljoin(response.url, location) if location else None',
    'while response.status_code == 202 or (response.status_code == 429 and recovery_url):',
    '    if response.status_code == 202 and not recovery_url:',
    '        raise RuntimeError("HTTP 202 response did not include Location")',
    '    if time.monotonic() >= deadline:',
    '        raise TimeoutError("Audio task did not finish within 15 minutes")',
    '    retry_after = int(response.headers.get("Retry-After", "2"))',
    '    time.sleep(retry_after)',
    '    response = requests.get(',
    '        recovery_url,',
    '        headers={"Authorization": f"Bearer {api_key}"},',
    '        timeout=420,',
    '    )',
    '    location = response.headers.get("Location")',
    '    if location:',
    '        recovery_url = urljoin(response.url, location)',
    '',
    'response.raise_for_status()',
    'content_type = response.headers.get("Content-Type", "").split(";", 1)[0].lower()',
    'if content_type != "audio/wav":',
    '    raise RuntimeError(f"Expected audio/wav, got {content_type or \'missing Content-Type\'}")',
    'Path("speech.wav").write_bytes(response.content)',
    'print("Saved speech.wav")',
  ].join('\n')
}

function buildNodeSample(
  language: 'typescript' | 'javascript',
  context: AudioSpeechSampleContext
): string {
  const submitUrl = `${context.baseUrl}${context.endpointPath}`
  const isTypeScript = language === 'typescript'
  const responseDeclaration = isTypeScript
    ? 'let response: Response'
    : 'let response'
  const sleepDeclaration = isTypeScript
    ? 'const sleep = (milliseconds: number) =>\n  new Promise<void>((resolve) => setTimeout(resolve, milliseconds))'
    : 'const sleep = (milliseconds) =>\n  new Promise((resolve) => setTimeout(resolve, milliseconds))'
  const recoveryDeclaration = isTypeScript
    ? 'let recoveryUrl: URL | undefined'
    : 'let recoveryUrl'
  const recoveryUrlArgument = isTypeScript ? 'recoveryUrl!' : 'recoveryUrl'

  return [
    "import { randomUUID } from 'node:crypto'",
    "import { writeFile } from 'node:fs/promises'",
    '',
    `const apiKey = process.env.${context.apiKeyEnv}`,
    `if (!apiKey) throw new Error('Set ${context.apiKeyEnv} before running')`,
    '',
    sleepDeclaration,
    '',
    `const submitUrl = '${submitUrl}'`,
    `${responseDeclaration} = await fetch(submitUrl, {`,
    "  method: 'POST',",
    '  headers: {',
    '    Authorization: `Bearer ${apiKey}`,',
    "    'Content-Type': 'application/json',",
    "    'Idempotency-Key': `indextts2-${randomUUID()}`,",
    '  },',
    '  body: JSON.stringify({',
    `    model: '${context.modelName}',`,
    '    emo_sad: 0,',
    '    emo_calm: 0.3,',
    '    emo_angry: 0,',
    '    emo_happy: 0.5,',
    '    emo_afraid: 0,',
    '    emo_random: false,',
    "    prompt_text: '你好，这是一段 IndexTTS2 语音合成示例。',",
    '    emo_disgusted: 0,',
    `    emo_ref_audio: '${EMOTION_AUDIO_PLACEHOLDER}', // Replace before running.`,
    "    emo_surprised: '0',",
    `    prompt_simple: '${REFERENCE_AUDIO_PLACEHOLDER}', // Replace before running.`,
    '    emo_melancholic: 0,',
    "    emo_control_method: '与音色参考音频相同',",
    '  }),',
    '  signal: AbortSignal.timeout(420_000),',
    '})',
    '',
    'const deadline = Date.now() + 900_000',
    recoveryDeclaration,
    "const initialLocation = response.headers.get('Location')",
    'if (initialLocation) recoveryUrl = new URL(initialLocation, response.url)',
    'while (response.status === 202 || (response.status === 429 && recoveryUrl)) {',
    "  if (response.status === 202 && !recoveryUrl) throw new Error('HTTP 202 response did not include Location')",
    '  if (Date.now() >= deadline) {',
    "    throw new Error('Audio task did not finish within 15 minutes')",
    '  }',
    "  const retryAfter = Number(response.headers.get('Retry-After') ?? 2)",
    '  await sleep(retryAfter * 1_000)',
    `  response = await fetch(${recoveryUrlArgument}, {`,
    '    headers: { Authorization: `Bearer ${apiKey}` },',
    '    signal: AbortSignal.timeout(420_000),',
    '  })',
    "  const nextLocation = response.headers.get('Location')",
    '  if (nextLocation) recoveryUrl = new URL(nextLocation, response.url)',
    '}',
    '',
    'if (!response.ok) {',
    '  throw new Error(`Audio request failed (${response.status}): ${await response.text()}`)',
    '}',
    "const contentType = response.headers.get('Content-Type')?.split(';', 1)[0].toLowerCase()",
    "if (contentType !== 'audio/wav') {",
    "  throw new Error(`Expected audio/wav, got ${contentType ?? 'missing Content-Type'}`)",
    '}',
    "await writeFile('speech.wav', Buffer.from(await response.arrayBuffer()))",
    "console.log('Saved speech.wav')",
  ].join('\n')
}

export function buildIndexTTSAudioSpeechSample(
  language: ImageSampleLanguage,
  context: AudioSpeechSampleContext
): string {
  if (language === 'curl' || language === 'bash') {
    return buildCurlSample(context)
  }
  if (language === 'python') {
    return buildPythonSample(context)
  }
  return buildNodeSample(language, context)
}

export function buildIndexTTSAudioSpeechAiIntegrationGuide(
  context: AudioSpeechSampleContext
): string {
  return [
    '# Opwan IndexTTS2 speech API integration guide',
    '',
    'Use this document as the source of truth when implementing the integration.',
    '',
    '## Request contract',
    `- Submit: POST ${context.baseUrl}${context.endpointPath}`,
    `- Model: ${context.modelName}`,
    `- Authentication: Authorization: Bearer $${context.apiKeyEnv}`,
    '- Content-Type: application/json only.',
    '- Idempotency-Key is required and must be no longer than 256 characters.',
    '- The complete model contract uses model, prompt_text, prompt_simple, and emo_control_method. input and voice remain accepted compatibility aliases for prompt_text and prompt_simple.',
    '- prompt_simple is reference speaker audio, not a named OpenAI voice. It accepts a public HTTPS WAV/MP3 URL or a matching base64 data URI.',
    '',
    '## Response and recovery',
    '- HTTP 200 returns binary audio/wav. Read bytes or an ArrayBuffer; do not parse it as JSON.',
    '- If processing exceeds the synchronous wait, HTTP 202 returns task_id and status. Follow Location with an authenticated GET after Retry-After.',
    '- If the initial POST returns HTTP 429 with Location, the task was accepted. Wait for Retry-After and recover it with an authenticated GET; only a POST 429 without Location is a normal submission limit.',
    `- Recovery: GET ${context.baseUrl}${context.endpointPath}/{task_id} with the same API token.`,
    '- If a recovery GET returns HTTP 429, wait for Retry-After and retry the same recovery URL.',
    '- Reusing the same Idempotency-Key with the same canonical request replays the original task without another charge.',
    '- Reusing the key with a different request returns HTTP 409 with code idempotency_conflict.',
    '',
    '## Limits',
    '- prompt_text must contain 1 to 2048 UTF-8 characters.',
    '- The complete JSON request body is limited to 64 MiB.',
    '- Each reference audio input is limited to 15 MiB; prompt_simple plus emo_ref_audio is limited to 30 MiB decoded.',
    '- Reference WAV/MP3 duration is limited to 10 minutes. Generated WAV output is limited to 64 MiB.',
    '',
    '## Emotion controls',
    '- emo_control_method currently accepts only 与音色参考音频相同.',
    '- emo_ref_audio accepts the same public HTTPS/data-URI audio formats as prompt_simple.',
    '- emo_happy, emo_angry, emo_sad, emo_afraid, emo_disgusted, emo_melancholic, and emo_calm each accept 0 to 1.4.',
    '- emo_surprised currently accepts only the string "0". emo_random accepts a boolean and defaults to false.',
    '- The earlier metadata.emotion_audio, metadata.emotion_vector, and metadata.emotion_random aliases remain accepted for compatibility but cannot be mixed with direct emo_* fields.',
    '',
    '## Unsupported options',
    '- instructions is unsupported.',
    '- response_format values other than wav and speed values other than 1 are rejected.',
    '- stream_format=sse is unsupported.',
    '',
    '## Quick start',
    '```bash',
    buildCurlSample(context),
    '```',
  ].join('\n')
}
