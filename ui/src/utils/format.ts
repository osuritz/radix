/**
 * Shared formatting utilities used across KPI cards and detail panels.
 * Centralising here ensures all views agree on formatting (e.g. sub-ms values).
 */

/**
 * Format a millisecond value into a human-readable latency string.
 *
 * - 0      → "0ms"   (never "0µs")
 * - <1 ms  → µs (e.g. 0.5ms → "500µs")
 * - <1000ms→ ms (e.g. 42.3ms)
 * - ≥1000ms→ seconds (e.g. 1.23s)
 */
export function formatMs(ms: number): string {
  if (ms === 0) return '0ms'
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`
  if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`
  return `${ms.toFixed(1)}ms`
}

/** Units table extended to PB and EB to prevent index overflow. */
const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB'] as const

/**
 * Format a byte count into a human-readable string.
 *
 * - Negative values    → "—"
 * - 0                  → "0 B"
 * - 0 < bytes < 1      → "0.50 B" (unit index clamped to 0 so fractional
 *                         averages never produce "512 undefined")
 * - Unit index clamped to EB so values ≥ 1 PB don't render a wrong unit.
 */
export function humanBytes(bytes: number): string {
  if (bytes < 0) return '—'
  if (bytes === 0) return '0 B'
  const i = Math.min(
    Math.max(Math.floor(Math.log(bytes) / Math.log(1024)), 0),
    BYTE_UNITS.length - 1
  )
  const val = bytes / Math.pow(1024, i)
  const formatted =
    val < 10 ? val.toFixed(2) : val < 100 ? val.toFixed(1) : val.toFixed(0)
  return `${formatted} ${BYTE_UNITS[i]}`
}

/**
 * Format an uptime value in seconds into a compact human-readable string.
 *
 * - <60s → "42s"
 * - <1h  → "3m 15s"
 * - ≥1h  → "2h 7m"
 */
export function formatUptime(seconds: number): string {
  if (seconds < 60) return `${Math.floor(seconds)}s`
  if (seconds < 3600) {
    const m = Math.floor(seconds / 60)
    const s = Math.floor(seconds % 60)
    return `${m}m ${s}s`
  }
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}h ${m}m`
}
