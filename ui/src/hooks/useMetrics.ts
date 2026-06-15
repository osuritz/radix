import { useState, useEffect, useRef, useCallback } from 'react'
import type { MetricsSnapshot } from '@/types/metrics'

/** Poll interval in milliseconds */
export const POLL_INTERVAL_MS = 2000

/** Number of samples to keep in the rolling ring buffer (2 min at 2s) */
export const RING_BUFFER_SIZE = 60

export interface HistoryPoint {
  /** Timestamp (ms since epoch) when this sample was captured */
  t: number;
  ratePerSecond: number;
  total: number;
  errors: number;
}

export interface UseMetricsResult {
  snapshot: MetricsSnapshot | null;
  history: HistoryPoint[];
  error: string | null;
  /** true when polling is up and last fetch succeeded */
  live: boolean;
}

export function useMetrics(): UseMetricsResult {
  const [snapshot, setSnapshot] = useState<MetricsSnapshot | null>(null)
  const [history, setHistory] = useState<HistoryPoint[]>([])
  const [error, setError] = useState<string | null>(null)
  const [live, setLive] = useState(false)

  // Ring buffer stored in a ref to avoid stale closures
  const ringRef = useRef<HistoryPoint[]>([])

  const fetchMetrics = useCallback(async (signal: AbortSignal) => {
    try {
      const res = await fetch('/_metrics', { signal })
      if (!res.ok) throw new Error(`HTTP ${res.status} ${res.statusText}`)
      const data = (await res.json()) as MetricsSnapshot

      setSnapshot(data)
      setError(null)
      setLive(true)

      // Append to ring buffer
      const point: HistoryPoint = {
        t: Date.now(),
        ratePerSecond: data.requests.rate_per_second,
        total: data.requests.total,
        errors: data.requests.errors,
      }
      ringRef.current = [
        ...ringRef.current.slice(-(RING_BUFFER_SIZE - 1)),
        point,
      ]
      setHistory([...ringRef.current])
    } catch (err) {
      if ((err as Error).name === 'AbortError') return
      const msg = err instanceof Error ? err.message : String(err)
      setError(msg)
      setLive(false)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    let timerId: ReturnType<typeof setTimeout> | undefined = undefined

    const poll = () => {
      fetchMetrics(controller.signal).finally(() => {
        // Schedule next poll only if not aborted
        if (!controller.signal.aborted) {
          timerId = setTimeout(poll, POLL_INTERVAL_MS)
        }
      })
    }

    // Kick off immediately
    poll()

    return () => {
      controller.abort()
      clearTimeout(timerId)
    }
  }, [fetchMetrics])

  return { snapshot, history, error, live }
}
