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

/** Pick a color for a status text key */
function statusColor(key: string): string {
  const lower = key.toLowerCase()
  if (lower.includes('not found') || lower.includes('unauthorized') || lower.includes('forbidden')) {
    return 'var(--ctp-yellow)'
  }
  if (lower.includes('error') || lower.includes('bad') || lower.includes('gateway') || lower.includes('timeout')) {
    return 'var(--ctp-red)'
  }
  if (lower.includes('created') || lower.includes('accepted') || lower.includes('no content')) {
    return 'var(--ctp-green)'
  }
  if (lower.includes('redirect') || lower.includes('moved') || lower.includes('found')) {
    return 'var(--ctp-lavender)'
  }
  // "OK", "Continue", etc.
  return 'var(--ctp-blue)'
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
