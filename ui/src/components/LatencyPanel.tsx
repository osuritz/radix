import type { ResponseTimesMetrics } from '@/types/metrics'
import { formatMs } from '@/utils/format'

interface LatencyPanelProps {
  responseTimes: ResponseTimesMetrics;
}

/** Map a ms value to a health color: green → yellow → red */
function latencyColor(ms: number): string {
  if (ms < 100) return 'var(--ctp-green)'
  if (ms < 500) return 'var(--ctp-yellow)'
  return 'var(--ctp-red)'
}

interface StatRowProps {
  label: string;
  value: string;
  color: string;
}

function StatRow({ label, value, color }: StatRowProps) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span style={{ color: 'var(--ctp-subtext)' }} className="text-xs">{label}</span>
      <span style={{ color }} className="text-sm font-semibold tabular-nums">{value}</span>
    </div>
  )
}

export function LatencyPanel({ responseTimes }: LatencyPanelProps) {
  const { min_ms, max_ms, avg_ms, p50_ms, p95_ms, p99_ms } = responseTimes

  // Gauge bar: fill based on p95 relative to a reference of 1000ms (capped)
  const p95Pct = Math.min(100, (p95_ms / 1000) * 100)
  const gaugeColor = latencyColor(p95_ms)

  return (
    <div
      style={{ backgroundColor: 'var(--ctp-surface)', borderColor: 'var(--ctp-border)' }}
      className="rounded-lg border p-4 flex flex-col gap-3"
    >
      <h2 style={{ color: 'var(--ctp-text)' }} className="text-sm font-semibold uppercase tracking-wider">
        Latency
      </h2>

      {/* P95 range bar */}
      <div>
        <div className="flex justify-between mb-1">
          <span style={{ color: 'var(--ctp-subtext)' }} className="text-xs">p95</span>
          <span style={{ color: gaugeColor }} className="text-xs font-semibold tabular-nums">{formatMs(p95_ms)}</span>
        </div>
        <div
          style={{ backgroundColor: 'var(--ctp-base)', borderRadius: 4 }}
          className="w-full h-2 overflow-hidden"
        >
          <div
            style={{
              width: `${p95Pct}%`,
              backgroundColor: gaugeColor,
              height: '100%',
              borderRadius: 4,
              transition: 'width 0.4s ease',
            }}
          />
        </div>
        <div className="flex justify-between mt-0.5">
          <span style={{ color: 'var(--ctp-overlay)' }} className="text-xs">0</span>
          <span style={{ color: 'var(--ctp-overlay)' }} className="text-xs">1s</span>
        </div>
      </div>

      {/* Stat rows */}
      <div className="flex flex-col gap-1.5 mt-1">
        <StatRow label="p50" value={formatMs(p50_ms)} color={latencyColor(p50_ms)} />
        <StatRow label="p95" value={formatMs(p95_ms)} color={latencyColor(p95_ms)} />
        <StatRow label="p99" value={formatMs(p99_ms)} color={latencyColor(p99_ms)} />
        <div
          style={{ borderTopColor: 'var(--ctp-border)' }}
          className="border-t my-1"
        />
        <StatRow label="avg" value={formatMs(avg_ms)} color="var(--ctp-text)" />
        <StatRow label="min" value={formatMs(min_ms)} color="var(--ctp-green)" />
        <StatRow label="max" value={formatMs(max_ms)} color="var(--ctp-red)" />
      </div>
    </div>
  )
}
