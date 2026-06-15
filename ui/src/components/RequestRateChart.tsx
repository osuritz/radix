import { useId } from 'react'
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import type { HistoryPoint } from '@/hooks/useMetrics'

interface RequestRateChartProps {
  history: HistoryPoint[];
}

/** A single chart-ready data point produced by {@link toChartData}. */
export interface ChartDataPoint {
  time: string;
  rate: number;
  errorsPerSec: number;
}

function formatTime(t: number): string {
  const d = new Date(t)
  const h = d.getHours().toString().padStart(2, '0')
  const m = d.getMinutes().toString().padStart(2, '0')
  const s = d.getSeconds().toString().padStart(2, '0')
  return `${h}:${m}:${s}`
}

/**
 * Convert a history ring-buffer into chart-ready data points.
 *
 * - The first point always has `errorsPerSec = 0` (no previous sample to diff).
 * - Cumulative counter resets (server restart) are clamped to 0, never negative.
 *
 * Exported for unit-testing purposes; the component uses it internally.
 */
export function toChartData(history: HistoryPoint[]): ChartDataPoint[] {
  return history.map((p, i) => {
    let errPerSec = 0
    if (i > 0) {
      const prev = history[i - 1]
      const dtMs = p.t - prev.t
      if (dtMs > 0) {
        errPerSec = Math.max(0, (p.errors - prev.errors) / (dtMs / 1000))
      }
    }
    return {
      time: formatTime(p.t),
      rate: +p.ratePerSecond.toFixed(2),
      errorsPerSec: +errPerSec.toFixed(2),
    }
  })
}

export function RequestRateChart({ history }: RequestRateChartProps) {
  // Unique IDs per instance so SVG gradient ids don't collide if mounted twice.
  const uid = useId()
  const rateGradId = `rateGrad-${uid}`
  const errGradId = `errGrad-${uid}`

  const data = toChartData(history)

  return (
    <div
      style={{ backgroundColor: 'var(--ctp-surface)', borderColor: 'var(--ctp-border)' }}
      className="rounded-lg border p-4 flex flex-col gap-3"
    >
      <h2 style={{ color: 'var(--ctp-text)' }} className="text-sm font-semibold uppercase tracking-wider">
        Request Rate (req/s)
      </h2>
      {data.length < 2 ? (
        <div style={{ color: 'var(--ctp-overlay)' }} className="text-sm text-center py-8">
          Collecting data…
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={200}>
          <AreaChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
            <defs>
              {/* Use style={{ stopColor }} instead of stopColor="var()" so Firefox
                  and Safari<15.4 resolve the CSS variable in SVG presentation context. */}
              <linearGradient id={rateGradId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" style={{ stopColor: 'var(--ctp-blue)' }} stopOpacity={0.35} />
                <stop offset="95%" style={{ stopColor: 'var(--ctp-blue)' }} stopOpacity={0} />
              </linearGradient>
              <linearGradient id={errGradId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" style={{ stopColor: 'var(--ctp-red)' }} stopOpacity={0.3} />
                <stop offset="95%" style={{ stopColor: 'var(--ctp-red)' }} stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--ctp-border)" strokeOpacity={0.5} />
            <XAxis
              dataKey="time"
              tick={{ fontSize: 10, fill: 'var(--ctp-overlay)' }}
              interval="preserveStartEnd"
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tick={{ fontSize: 10, fill: 'var(--ctp-overlay)' }}
              tickLine={false}
              axisLine={false}
              width={32}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: 'var(--ctp-surface)',
                border: '1px solid var(--ctp-border)',
                borderRadius: '8px',
                color: 'var(--ctp-text)',
              }}
              labelStyle={{ color: 'var(--ctp-subtext)', fontSize: 11 }}
              itemStyle={{ color: 'var(--ctp-text)', fontSize: 12 }}
            />
            <Area
              type="monotone"
              dataKey="rate"
              name="req/s"
              stroke="var(--ctp-blue)"
              strokeWidth={2}
              fill={`url(#${rateGradId})`}
              dot={false}
              activeDot={{ r: 4, fill: 'var(--ctp-blue)' }}
            />
            <Area
              type="monotone"
              dataKey="errorsPerSec"
              name="errors/s"
              stroke="var(--ctp-red)"
              strokeWidth={1.5}
              fill={`url(#${errGradId})`}
              dot={false}
              activeDot={{ r: 3, fill: 'var(--ctp-red)' }}
            />
          </AreaChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}
