import { describe, it, expect } from 'vitest'
import { statusColor } from './StatusCodesChart'

/**
 * Unit tests for the statusColor helper extracted from StatusCodesChart.
 * Verifies the HTTP class → CSS variable mapping without rendering the chart.
 */
describe('statusColor', () => {
  describe('2xx — success → green', () => {
    it.each(['OK', 'Created', 'Accepted', 'No Content'])(
      'statusColor("%s") returns the green CSS variable',
      (phrase) => {
        expect(statusColor(phrase)).toBe('var(--ctp-green)')
      }
    )
  })

  describe('3xx — redirect → lavender', () => {
    it.each([
      'Moved Permanently',
      'Found',
      'Not Modified',
      'Temporary Redirect',
      'Permanent Redirect',
    ])('statusColor("%s") returns the lavender CSS variable', (phrase) => {
      expect(statusColor(phrase)).toBe('var(--ctp-lavender)')
    })
  })

  describe('4xx — client error → yellow', () => {
    it.each([
      'Not Found',
      'Method Not Allowed',
      'Bad Request',
      'Unauthorized',
      'Forbidden',
      'Conflict',
      'Gone',
      'Unprocessable Entity',
      'Too Many Requests',
    ])('statusColor("%s") returns the yellow CSS variable', (phrase) => {
      expect(statusColor(phrase)).toBe('var(--ctp-yellow)')
    })
  })

  describe('5xx — server error → red', () => {
    it.each([
      'Service Unavailable',
      'Internal Server Error',
      'Not Implemented',
      'Bad Gateway',
      'Gateway Timeout',
    ])('statusColor("%s") returns the red CSS variable', (phrase) => {
      expect(statusColor(phrase)).toBe('var(--ctp-red)')
    })
  })

  describe('unknown phrase → neutral overlay (not the OK color)', () => {
    it.each([
      'I Am A Teapot',
      '',
      'Foo Bar',
      'Gateway timeout', // wrong capitalisation
      '200',
    ])('statusColor("%s") returns overlay and NOT green', (phrase) => {
      const color = statusColor(phrase)
      expect(color).toBe('var(--ctp-overlay)')
      expect(color).not.toBe('var(--ctp-green)')
    })
  })
})
