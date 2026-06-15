import { useColorScheme } from '@/hooks/color-scheme/color-scheme'
import { useMetrics } from '@/hooks/useMetrics'
import { Header } from '@/components/Header'
import { KpiCard } from '@/components/KpiCard'
import { RequestRateChart } from '@/components/RequestRateChart'
import { StatusCodesChart } from '@/components/StatusCodesChart'
import { LatencyPanel } from '@/components/LatencyPanel'
import { BandwidthPanel } from '@/components/BandwidthPanel'
import { CommandStats } from '@/components/CommandStats'
import { formatMs, formatUptime } from '@/utils/format'

/** Commands that have per-command stats panels. */
const COMMAND_STATS_COMMANDS = new Set(['echo', 'mock', 'proxy'])

function ErrorBanner({ message }: { message: string }) {
  return (
    <div
      style={{
        backgroundColor: 'color-mix(in srgb, var(--ctp-red) 15%, transparent)',
        borderColor: 'var(--ctp-red)',
        color: 'var(--ctp-red)',
      }}
      className="border rounded-lg px-4 py-3 text-sm flex items-center gap-2"
      role="alert"
    >
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
        <circle cx="12" cy="12" r="10" />
        <line x1="12" y1="8" x2="12" y2="12" />
        <line x1="12" y1="16" x2="12.01" y2="16" />
      </svg>
      <span>Unable to reach /_metrics: {message}. Retrying…</span>
    </div>
  )
}

export default function App() {
  const { colorScheme, setColorScheme } = useColorScheme()
  const { snapshot, history, error, live } = useMetrics()

  const req = snapshot?.requests
  const rt = snapshot?.response_times
  const bw = snapshot?.bandwidth
  const srv = snapshot?.server

  return (
    <div style={{ minHeight: '100vh', backgroundColor: 'var(--ctp-base)' }}>
      <Header
        snapshot={snapshot}
        live={live}
        colorScheme={colorScheme}
        setColorScheme={setColorScheme}
      />

      <main className="max-w-7xl mx-auto px-4 py-6 flex flex-col gap-6">
        {/* Error banner */}
        {error && <ErrorBanner message={error} />}

        {/* Loading state */}
        {!snapshot && !error && (
          <div style={{ color: 'var(--ctp-subtext)' }} className="text-center py-16 text-sm">
            Connecting to /_metrics…
          </div>
        )}

        {snapshot && (
          <>
            {/* KPI Row */}
            <section aria-label="Key performance indicators" className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
              <KpiCard
                label="Total Requests"
                value={(req?.total ?? 0).toLocaleString()}
                subValue={`${(req?.success ?? 0).toLocaleString()} success`}
                accent="blue"
              />
              <KpiCard
                label="Errors"
                value={(req?.errors ?? 0).toLocaleString()}
                subValue={
                  req && req.total > 0
                    ? `${((req.errors / req.total) * 100).toFixed(1)}% error rate`
                    : undefined
                }
                accent={req && req.errors > 0 ? 'red' : 'green'}
              />
              <KpiCard
                label="p50 Latency"
                value={rt ? formatMs(rt.p50_ms) : '—'}
                subValue={rt ? `p99: ${formatMs(rt.p99_ms)}` : undefined}
                accent="lavender"
              />
              <KpiCard
                label="Req / s"
                value={(req?.rate_per_second ?? 0).toFixed(2)}
                subValue="rolling average"
                accent="yellow"
              />
              <KpiCard
                label="Uptime"
                value={formatUptime(srv?.uptime_seconds ?? 0)}
                subValue={srv?.start_time ? new Date(srv.start_time).toLocaleTimeString() : undefined}
                accent="green"
              />
            </section>

            {/* Charts Row */}
            <section aria-label="Charts" className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <RequestRateChart history={history} />
              <StatusCodesChart statusCodes={snapshot.status_codes} />
            </section>

            {/* Stats Row */}
            <section aria-label="Latency and bandwidth" className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {rt && <LatencyPanel responseTimes={rt} />}
              {bw && <BandwidthPanel bandwidth={bw} />}
            </section>

            {/* Command-specific stats — only rendered for commands that have panels */}
            {snapshot.command && srv?.command && COMMAND_STATS_COMMANDS.has(srv.command) && (
              <section aria-label="Command statistics">
                <CommandStats command={snapshot.command} commandName={snapshot.server.command} />
              </section>
            )}
          </>
        )}
      </main>
    </div>
  )
}
