/**
 * Unit tests for the useMetrics hook.
 *
 * Strategy:
 * - Mock the global `fetch` with vi.stubGlobal so we control every response.
 * - Use fake timers so the self-scheduling setTimeout never fires unexpectedly.
 * - `settlePoll()` drives exactly one poll cycle: it fires any pending timers
 *   once and flushes the resulting promise chain, without triggering the
 *   recursive setTimeout that would otherwise loop infinitely under
 *   vi.runAllTimersAsync.
 * - renderHook + act from @testing-library/react drive React state updates.
 * - Stubs and timers are restored in afterEach to prevent test bleed.
 */

import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useMetrics, RING_BUFFER_SIZE, POLL_INTERVAL_MS } from './useMetrics'
import type { MetricsSnapshot } from '@/types/metrics'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Flush all pending microtasks (resolved promise callbacks). */
const flushMicrotasks = () => act(async () => { await Promise.resolve() })

/**
 * Drive exactly ONE poll cycle:
 *  1. Fire any pending timer (the setTimeout that kicked off the poll).
 *  2. Flush the resulting promise chain so fetch + state updates settle.
 *
 * This avoids vi.runAllTimersAsync which would spin the reschedule loop.
 */
async function settlePoll(): Promise<void> {
  // Step 1: advance time by 0 to trigger the pending setTimeout callback
  // without advancing to the NEXT poll interval.
  await act(async () => {
    vi.advanceTimersByTime(0)
  })
  // Step 2: flush the microtask queue so the fetch promise and setState calls
  // complete before we inspect result.current.
  await flushMicrotasks()
  await flushMicrotasks() // two rounds covers promise.then().finally() chains
}

/** Minimal valid MetricsSnapshot for tests that need a realistic payload. */
function makeSnapshot(overrides?: Partial<MetricsSnapshot['requests']>): MetricsSnapshot {
  return {
    server: {
      command: 'serve',
      uptime_seconds: 120,
      start_time: '2026-01-01T00:00:00Z',
      version: 'v0.7.1',
    },
    requests: {
      total: 10,
      success: 9,
      errors: 1,
      rate_per_second: 0.5,
      ...overrides,
    },
    status_codes: { OK: 9, 'Internal Server Error': 1 },
    methods: { GET: 10 },
    response_times: {
      min_ms: 1,
      max_ms: 100,
      avg_ms: 20,
      p50_ms: 15,
      p95_ms: 80,
      p99_ms: 95,
      count: 10,
    },
    bandwidth: {
      bytes_sent: 1024,
      bytes_received: 512,
    },
  }
}

/**
 * Build a minimal Response for vi.stubGlobal('fetch', ...).
 * We only implement the fields/methods the hook actually calls.
 */
function mockJsonResponse(body: unknown, status = 200): Response {
  const json = JSON.stringify(body)
  return new Response(json, {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function mockPlainTextResponse(body: string, status = 200): Response {
  return new Response(body, {
    status,
    headers: { 'Content-Type': 'text/plain; version=0.0.4' },
  })
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useMetrics', () => {
  let fetchMock: Mock

  beforeEach(() => {
    vi.useFakeTimers()
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('starts with snapshot=null, history=[], error=null, live=false', () => {
    // fetch never resolves — hook stays in initial state
    fetchMock.mockReturnValue(new Promise(() => {}))
    const { result } = renderHook(() => useMetrics())
    expect(result.current.snapshot).toBeNull()
    expect(result.current.history).toEqual([])
    expect(result.current.error).toBeNull()
    expect(result.current.live).toBe(false)
  })

  it('(a) a JSON 200 response sets snapshot, appends a history point, clears error, sets live', async () => {
    const snapshot = makeSnapshot()
    fetchMock.mockResolvedValue(mockJsonResponse(snapshot))

    const { result } = renderHook(() => useMetrics())
    await settlePoll()

    expect(result.current.snapshot).toEqual(snapshot)
    expect(result.current.error).toBeNull()
    expect(result.current.live).toBe(true)
    expect(result.current.history).toHaveLength(1)

    const pt = result.current.history[0]
    expect(pt.ratePerSecond).toBe(snapshot.requests.rate_per_second)
    expect(pt.total).toBe(snapshot.requests.total)
    expect(pt.errors).toBe(snapshot.requests.errors)
    expect(typeof pt.t).toBe('number')
  })

  it('(b) a 200 with text/plain Content-Type sets a clear error and leaves snapshot null', async () => {
    fetchMock.mockResolvedValue(
      mockPlainTextResponse(
        '# TYPE radix_requests_total counter\nradix_requests_total 42\n'
      )
    )

    const { result } = renderHook(() => useMetrics())
    await settlePoll()

    expect(result.current.snapshot).toBeNull()
    expect(result.current.live).toBe(false)
    // The error message instructs the user to switch to JSON mode and reports
    // what the server actually returned — both signals must be present.
    expect(result.current.error).toContain('json')
    expect(result.current.error).toContain('text/plain')
  })

  it('sets error on a non-2xx HTTP response', async () => {
    fetchMock.mockResolvedValue(
      new Response('Not Found', { status: 404, statusText: 'Not Found' })
    )

    const { result } = renderHook(() => useMetrics())
    await settlePoll()

    expect(result.current.error).toContain('HTTP 404')
    expect(result.current.live).toBe(false)
    expect(result.current.snapshot).toBeNull()
  })

  it('sets error on network failure (fetch throws)', async () => {
    fetchMock.mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(() => useMetrics())
    await settlePoll()

    expect(result.current.error).toContain('Network error')
    expect(result.current.live).toBe(false)
  })

  it('(c) history ring buffer never exceeds RING_BUFFER_SIZE across multiple polls', async () => {
    const snapshot = makeSnapshot()
    fetchMock.mockResolvedValue(mockJsonResponse(snapshot))

    const { result } = renderHook(() => useMetrics())

    // Drive RING_BUFFER_SIZE + 5 poll cycles manually.
    const extra = 5
    const totalPolls = RING_BUFFER_SIZE + extra

    for (let i = 0; i < totalPolls; i++) {
      await settlePoll()
      // Advance time to trigger the next scheduled poll
      await act(async () => {
        vi.advanceTimersByTime(POLL_INTERVAL_MS)
      })
    }

    expect(result.current.history.length).toBeLessThanOrEqual(RING_BUFFER_SIZE)
  })

  it('aborts in-flight fetch on unmount — abort signal is set, no state update after unmount', async () => {
    let capturedSignal: AbortSignal | undefined
    fetchMock.mockImplementation((_url: string, opts: RequestInit) => {
      capturedSignal = opts?.signal as AbortSignal | undefined
      // Never resolves — simulates a slow/pending fetch
      return new Promise(() => {})
    })

    const { unmount } = renderHook(() => useMetrics())

    await act(async () => {
      unmount()
    })

    expect(capturedSignal?.aborted).toBe(true)
  })
})
