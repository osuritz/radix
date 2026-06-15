import { describe, it, expect } from 'vitest'
import { toChartData } from './RequestRateChart'
import type { HistoryPoint } from '@/hooks/useMetrics'

function makePoint(
  t: number,
  errors: number,
  ratePerSecond = 0,
  total = 0
): HistoryPoint {
  return { t, errors, ratePerSecond, total }
}

describe('toChartData', () => {
  it('returns an empty array for empty history', () => {
    expect(toChartData([])).toEqual([])
  })

  it('first point always has errorsPerSec === 0 (no previous sample)', () => {
    const history = [makePoint(1000, 5, 1.2)]
    const [p] = toChartData(history)
    expect(p.errorsPerSec).toBe(0)
    expect(p.rate).toBe(1.2)
  })

  it('computes per-second error deltas from cumulative counters', () => {
    // 10 errors over 2 seconds → 5 errors/s
    const history = [
      makePoint(0, 0, 0),
      makePoint(2000, 10, 3),
    ]
    const pts = toChartData(history)
    expect(pts[0].errorsPerSec).toBe(0)
    expect(pts[1].errorsPerSec).toBe(5)
    expect(pts[1].rate).toBe(3)
  })

  it('clamps negative delta (counter reset) to 0 — never negative', () => {
    // Simulate server restart: error counter goes from 100 back to 2
    const history = [
      makePoint(0, 100, 0),
      makePoint(1000, 2, 0),
    ]
    const pts = toChartData(history)
    expect(pts[1].errorsPerSec).toBe(0)
  })

  it('handles a sequence of 3+ points, accumulating correct deltas', () => {
    const history = [
      makePoint(0, 0, 0),
      makePoint(1000, 5, 2),  // +5 errors in 1 s → 5 err/s
      makePoint(3000, 9, 4),  // +4 errors in 2 s → 2 err/s
    ]
    const [p0, p1, p2] = toChartData(history)
    expect(p0.errorsPerSec).toBe(0)
    expect(p1.errorsPerSec).toBe(5)
    expect(p2.errorsPerSec).toBe(2)
  })

  it('rounds errorsPerSec to 2 decimal places', () => {
    // 1 error over 3 seconds ≈ 0.333... → rounds to 0.33
    const history = [
      makePoint(0, 0, 0),
      makePoint(3000, 1, 0),
    ]
    const pts = toChartData(history)
    expect(pts[1].errorsPerSec).toBe(0.33)
  })

  it('produces a time string in HH:MM:SS format', () => {
    // Use a fixed epoch so the output is deterministic (local-timezone independent
    // only for format shape — we validate pattern, not exact value)
    const history = [makePoint(Date.now(), 0, 0)]
    const [p] = toChartData(history)
    expect(p.time).toMatch(/^\d{2}:\d{2}:\d{2}$/)
  })
})
