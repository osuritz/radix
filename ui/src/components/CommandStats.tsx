import type { CommandMetrics } from '@/types/metrics'

interface CommandStatsProps {
  command: CommandMetrics;
  commandName: string;
}

interface StatCellProps {
  label: string;
  value: number;
  accent?: string;
}

function StatCell({ label, value, accent = 'var(--ctp-blue)' }: StatCellProps) {
  return (
    <div
      style={{ backgroundColor: 'var(--ctp-base)', borderColor: 'var(--ctp-border)', borderTopColor: accent }}
      className="rounded border border-t-2 p-3 flex flex-col gap-1"
    >
      <span style={{ color: 'var(--ctp-subtext)' }} className="text-xs uppercase tracking-wide leading-tight">
        {label}
      </span>
      <span style={{ color: 'var(--ctp-text)' }} className="text-xl font-bold tabular-nums">
        {value.toLocaleString()}
      </span>
    </div>
  )
}

export function CommandStats({ command, commandName }: CommandStatsProps) {
  return (
    <div
      style={{ backgroundColor: 'var(--ctp-surface)', borderColor: 'var(--ctp-border)' }}
      className="rounded-lg border p-4 flex flex-col gap-4"
    >
      <h2 style={{ color: 'var(--ctp-text)' }} className="text-sm font-semibold uppercase tracking-wider">
        {commandName} stats
      </h2>

      {command.echo && (
        <div className="grid grid-cols-3 gap-3">
          <StatCell label="Delays applied" value={command.echo.delays_applied} accent="var(--ctp-lavender)" />
          <StatCell label="Custom body" value={command.echo.custom_body_responses} accent="var(--ctp-blue)" />
          <StatCell label="Path status hits" value={command.echo.path_status_hits} accent="var(--ctp-green)" />
        </div>
      )}

      {command.mock && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <StatCell label="Built-in routes" value={command.mock.route_matches_builtin} accent="var(--ctp-blue)" />
          <StatCell label="Custom routes" value={command.mock.route_matches_custom} accent="var(--ctp-lavender)" />
          <StatCell label="Template renders" value={command.mock.template_renders} accent="var(--ctp-green)" />
          <StatCell label="Template errors" value={command.mock.template_errors} accent="var(--ctp-red)" />
          <StatCell label="Reloads" value={command.mock.reloads} accent="var(--ctp-yellow)" />
          <StatCell label="Fail injections" value={command.mock.fail_injections} accent="var(--ctp-red)" />
          <StatCell label="404 fallbacks" value={command.mock.fallback_not_found} accent="var(--ctp-overlay)" />
          <StatCell label="Proxy fallbacks" value={command.mock.fallback_proxy} accent="var(--ctp-overlay)" />
        </div>
      )}

      {command.proxy && (
        <div className="grid grid-cols-2 gap-3">
          <StatCell label="Auth injections" value={command.proxy.auth_injections} accent="var(--ctp-green)" />
          <StatCell label="Stream connections" value={command.proxy.stream_connections} accent="var(--ctp-blue)" />
        </div>
      )}
    </div>
  )
}
