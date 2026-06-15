import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Cell,
  ResponsiveContainer,
} from 'recharts'
import type { StatusCodesMap } from '@/types/metrics'

interface StatusCodesChartProps {
  statusCodes: StatusCodesMap;
}

/**
 * Maps canonical Go http.StatusText() reason phrases to a color by HTTP class.
 * 2xx → green, 3xx → lavender, 4xx → yellow, 5xx → red.
 * Unknown phrases default to neutral overlay (NOT the OK color).
 */
const STATUS_COLORS: Record<string, string> = {
  // 2xx — success
  'OK': 'var(--ctp-green)',
  'Created': 'var(--ctp-green)',
  'Accepted': 'var(--ctp-green)',
  'No Content': 'var(--ctp-green)',
  // 3xx — redirect
  'Moved Permanently': 'var(--ctp-lavender)',
  'Found': 'var(--ctp-lavender)',
  'Not Modified': 'var(--ctp-lavender)',
  'Temporary Redirect': 'var(--ctp-lavender)',
  'Permanent Redirect': 'var(--ctp-lavender)',
  // 4xx — client error
  'Bad Request': 'var(--ctp-yellow)',
  'Unauthorized': 'var(--ctp-yellow)',
  'Forbidden': 'var(--ctp-yellow)',
  'Not Found': 'var(--ctp-yellow)',
  'Method Not Allowed': 'var(--ctp-yellow)',
  'Conflict': 'var(--ctp-yellow)',
  'Gone': 'var(--ctp-yellow)',
  'Unprocessable Entity': 'var(--ctp-yellow)',
  'Too Many Requests': 'var(--ctp-yellow)',
  // 5xx — server error
  'Internal Server Error': 'var(--ctp-red)',
  'Not Implemented': 'var(--ctp-red)',
  'Bad Gateway': 'var(--ctp-red)',
  'Service Unavailable': 'var(--ctp-red)',
  'Gateway Timeout': 'var(--ctp-red)',
}

/**
 * Pick a color for a status text key via the canonical phrase map.
 * Exported for unit-testing purposes; the component uses it internally.
 */
export function statusColor(key: string): string {
  return STATUS_COLORS[key] ?? 'var(--ctp-overlay)'
}

export function StatusCodesChart({ statusCodes }: StatusCodesChartProps) {
  const data = Object.entries(statusCodes)
    .sort((a, b) => b[1] - a[1])
    .map(([name, count]) => ({ name, count }))

  const isEmpty = data.length === 0

  return (
    <div
      style={{ backgroundColor: 'var(--ctp-surface)', borderColor: 'var(--ctp-border)' }}
      className="rounded-lg border p-4 flex flex-col gap-3"
    >
      <h2 style={{ color: 'var(--ctp-text)' }} className="text-sm font-semibold uppercase tracking-wider">
        Status Codes
      </h2>
      {isEmpty ? (
        <div style={{ color: 'var(--ctp-overlay)' }} className="text-sm text-center py-8">
          No data yet
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={Math.max(160, data.length * 36)}>
          <BarChart
            data={data}
            layout="vertical"
            margin={{ top: 0, right: 8, bottom: 0, left: 0 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="var(--ctp-border)" strokeOpacity={0.4} horizontal={false} />
            <XAxis
              type="number"
              tick={{ fontSize: 10, fill: 'var(--ctp-overlay)' }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              type="category"
              dataKey="name"
              tick={{ fontSize: 11, fill: 'var(--ctp-text)' }}
              tickLine={false}
              axisLine={false}
              width={120}
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
              cursor={{ fill: 'var(--ctp-overlay)', opacity: 0.1 }}
            />
            <Bar dataKey="count" name="requests" radius={[0, 4, 4, 0]}>
              {data.map((entry) => (
                <Cell key={entry.name} fill={statusColor(entry.name)} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}
