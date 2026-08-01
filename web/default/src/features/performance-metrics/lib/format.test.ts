import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { hasObservablePerformance } from './format'

describe('performance metric formatting', () => {
  test('does not present an empty async submission bucket as a measured failure', () => {
    assert.equal(
      hasObservablePerformance({
        avg_latency_ms: 0,
        avg_ttft_ms: 0,
        avg_tps: 0,
      }),
      false
    )
    assert.equal(
      hasObservablePerformance({
        avg_latency_ms: 87_000,
        avg_ttft_ms: 0,
        avg_tps: 0,
      }),
      true
    )
  })
})
