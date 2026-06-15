import { describe, it, expect } from 'vitest'
import { formatMs, humanBytes, formatUptime } from './format'

describe('formatMs', () => {
  it.each([
    { input: 0, expected: '0ms' },
    { input: 0.5, expected: '500µs' },
    { input: 0.001, expected: '1µs' },
    { input: 42.3, expected: '42.3ms' },
    { input: 999.9, expected: '999.9ms' },
    { input: 1000, expected: '1.00s' },
    { input: 1500, expected: '1.50s' },
    { input: 60000, expected: '60.00s' },
  ])('formatMs($input) → "$expected"', ({ input, expected }) => {
    expect(formatMs(input)).toBe(expected)
  })
})

describe('humanBytes', () => {
  it.each([
    // Edge: negative
    { input: -1, expected: '—' },
    { input: -0.001, expected: '—' },
    // Edge: zero
    { input: 0, expected: '0 B' },
    // Fractional bytes — the "512 undefined" regression: unit must be "B"
    { input: 0.5, expected: '0.50 B' },
    // Exact boundary values
    { input: 512, expected: '512 B' },
    { input: 1023, expected: '1023 B' },
    { input: 1024, expected: '1.00 KB' },
    { input: 1536, expected: '1.50 KB' },
    { input: 1024 * 1024, expected: '1.00 MB' },
    { input: 1024 ** 4, expected: '1.00 TB' },
    // PB value — clamped unit must be PB or EB, never undefined
    { input: 1024 ** 5, expected: '1.00 PB' },
    // >=1 PB — clamped to EB (last entry in the table)
    { input: 1024 ** 6, expected: '1.00 EB' },
  ])('humanBytes($input) → "$expected"', ({ input, expected }) => {
    expect(humanBytes(input)).toBe(expected)
  })

  it('never returns a string containing "undefined"', () => {
    const problematic = [0.5, 512, 1, 0.001, 1024 ** 5, 1024 ** 6]
    for (const v of problematic) {
      expect(humanBytes(v)).not.toContain('undefined')
    }
  })
})

describe('formatUptime', () => {
  it.each([
    { input: 0, expected: '0s' },
    { input: 42, expected: '42s' },
    { input: 59.9, expected: '59s' },
    { input: 60, expected: '1m 0s' },
    { input: 195, expected: '3m 15s' },
    { input: 3599, expected: '59m 59s' },
    { input: 3600, expected: '1h 0m' },
    { input: 7627, expected: '2h 7m' },
    { input: 86400, expected: '24h 0m' },
  ])('formatUptime($input) → "$expected"', ({ input, expected }) => {
    expect(formatUptime(input)).toBe(expected)
  })
})
