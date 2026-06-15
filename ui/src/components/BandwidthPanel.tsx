import type { BandwidthMetrics } from '@/types/metrics'
import { humanBytes } from '@/utils/format'

interface BandwidthPanelProps {
  bandwidth: BandwidthMetrics;
}

interface BwRowProps {
  label: string;
  value: string;
  color: string;
}

function BwRow({ label, value, color }: BwRowProps) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span style={{ color: 'var(--ctp-subtext)' }} className="text-xs">{label}</span>
      <span style={{ color }} className="text-sm font-semibold tabular-nums">{value}</span>
    </div>
  )
}

export function BandwidthPanel({ bandwidth }: BandwidthPanelProps) {
  const { bytes_sent, bytes_received, avg_request_size_bytes, avg_response_size_bytes } = bandwidth

  const total = bytes_sent + bytes_received

  return (
    <div
      style={{ backgroundColor: 'var(--ctp-surface)', borderColor: 'var(--ctp-border)' }}
      className="rounded-lg border p-4 flex flex-col gap-3"
    >
      <h2 style={{ color: 'var(--ctp-text)' }} className="text-sm font-semibold uppercase tracking-wider">
        Bandwidth
      </h2>

      {/* Split bar: sent vs received. Render a neutral empty bar when total === 0. */}
      <div>
        <div className="flex h-3 rounded-full overflow-hidden gap-px" style={{ backgroundColor: 'var(--ctp-base)' }}>
          {total === 0 ? (
            <div
              style={{ width: '100%', backgroundColor: 'var(--ctp-overlay)', opacity: 0.3 }}
              title="No data yet"
            />
          ) : (
            <>
              <div
                style={{
                  width: `${(bytes_sent / total) * 100}%`,
                  backgroundColor: 'var(--ctp-blue)',
                  transition: 'width 0.4s ease',
                }}
                title={`Sent: ${humanBytes(bytes_sent)}`}
              />
              <div
                style={{
                  width: `${(bytes_received / total) * 100}%`,
                  backgroundColor: 'var(--ctp-green)',
                  transition: 'width 0.4s ease',
                }}
                title={`Received: ${humanBytes(bytes_received)}`}
              />
            </>
          )}
        </div>
        <div className="flex justify-between mt-1">
          <div className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full" style={{ backgroundColor: 'var(--ctp-blue)' }} />
            <span style={{ color: 'var(--ctp-overlay)' }} className="text-xs">Sent</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full" style={{ backgroundColor: 'var(--ctp-green)' }} />
            <span style={{ color: 'var(--ctp-overlay)' }} className="text-xs">Received</span>
          </div>
        </div>
      </div>

      {/* Stats */}
      <div className="flex flex-col gap-1.5">
        <BwRow label="Total sent" value={humanBytes(bytes_sent)} color="var(--ctp-blue)" />
        <BwRow label="Total received" value={humanBytes(bytes_received)} color="var(--ctp-green)" />
        <div style={{ borderTopColor: 'var(--ctp-border)' }} className="border-t my-1" />
        {avg_response_size_bytes !== undefined && (
          <BwRow label="Avg response size" value={humanBytes(avg_response_size_bytes)} color="var(--ctp-subtext)" />
        )}
        {avg_request_size_bytes !== undefined && (
          <BwRow label="Avg request size" value={humanBytes(avg_request_size_bytes)} color="var(--ctp-subtext)" />
        )}
        {avg_request_size_bytes === undefined && avg_response_size_bytes === undefined && (
          <span style={{ color: 'var(--ctp-overlay)' }} className="text-xs text-center">No average data yet</span>
        )}
      </div>
    </div>
  )
}
