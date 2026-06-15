import type { ColorScheme, UserSpecifiedColorScheme } from '@/hooks/color-scheme/color-scheme'
import type { MetricsSnapshot } from '@/types/metrics'
import { POLL_INTERVAL_MS } from '@/hooks/useMetrics'

interface HeaderProps {
  snapshot: MetricsSnapshot | null;
  live: boolean;
  colorScheme: ColorScheme | null;
  setColorScheme: (value: UserSpecifiedColorScheme | null) => Promise<void>;
}

const commandColors: Record<string, { bg: string; text: string }> = {
  mock: { bg: 'var(--ctp-lavender)', text: 'var(--ctp-base)' },
  proxy: { bg: 'var(--ctp-blue)', text: 'var(--ctp-base)' },
  echo: { bg: 'var(--ctp-green)', text: 'var(--ctp-base)' },
  serve: { bg: 'var(--ctp-yellow)', text: 'var(--ctp-base)' },
}

/** Neutral color for unknown commands — does not imply "serve" yellow. */
const UNKNOWN_COMMAND_COLOR = { bg: 'var(--ctp-overlay)', text: 'var(--ctp-base)' }

export function Header({ snapshot, live, colorScheme, setColorScheme }: HeaderProps) {
  const command = snapshot?.server.command
  const cmdColors = command ? (commandColors[command] ?? UNKNOWN_COMMAND_COLOR) : null
  const refreshSecs = POLL_INTERVAL_MS / 1000

  const isDark = colorScheme === 'dark'

  function toggleColorScheme() {
    void setColorScheme(isDark ? 'light' : 'dark')
  }

  return (
    <header
      style={{
        backgroundColor: 'var(--ctp-surface)',
        borderBottomColor: 'var(--ctp-border)',
      }}
      className="border-b px-6 py-3 flex items-center gap-4"
    >
      {/* Logo + title */}
      <div className="flex items-center gap-2 flex-shrink-0">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <circle cx="12" cy="12" r="10" fill="var(--ctp-blue)" opacity="0.15" />
          <path
            d="M8 6h5a4 4 0 0 1 0 8H8V6z"
            fill="var(--ctp-blue)"
          />
          <path
            d="M8 14h4l4 4H12l-4-4z"
            fill="var(--ctp-lavender)"
          />
        </svg>
        <span style={{ color: 'var(--ctp-text)' }} className="font-semibold text-base tracking-tight">
          Radix Metrics
        </span>
      </div>

      {/* Command badge */}
      {command && cmdColors && (
        <span
          style={{ backgroundColor: cmdColors.bg, color: cmdColors.text }}
          className="text-xs font-bold px-2 py-0.5 rounded uppercase tracking-wider flex-shrink-0"
        >
          {command}
        </span>
      )}

      {/* Version */}
      {snapshot?.server.version && (
        <span style={{ color: 'var(--ctp-overlay)' }} className="text-xs flex-shrink-0">
          v{snapshot.server.version}
        </span>
      )}

      <div className="flex-1" />

      {/* Live indicator */}
      <div className="flex items-center gap-1.5 flex-shrink-0">
        <span
          className="w-2 h-2 rounded-full flex-shrink-0"
          style={{
            backgroundColor: live ? 'var(--ctp-green)' : 'var(--ctp-red)',
            boxShadow: live ? '0 0 6px var(--ctp-green)' : 'none',
          }}
          aria-label={live ? 'Live' : 'Disconnected'}
        />
        <span style={{ color: 'var(--ctp-subtext)' }} className="text-xs">
          {live ? `Live · every ${refreshSecs}s` : 'Disconnected'}
        </span>
      </div>

      {/* Theme toggle */}
      <button
        onClick={toggleColorScheme}
        aria-label={`Switch to ${isDark ? 'light' : 'dark'} theme`}
        style={{
          backgroundColor: 'var(--ctp-base)',
          borderColor: 'var(--ctp-border)',
          color: 'var(--ctp-subtext)',
        }}
        className="border rounded p-1.5 text-sm hover:opacity-80 transition-opacity flex-shrink-0"
      >
        {isDark ? (
          // Sun icon
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <circle cx="12" cy="12" r="4" />
            <line x1="12" y1="2" x2="12" y2="6" />
            <line x1="12" y1="18" x2="12" y2="22" />
            <line x1="4.93" y1="4.93" x2="7.76" y2="7.76" />
            <line x1="16.24" y1="16.24" x2="19.07" y2="19.07" />
            <line x1="2" y1="12" x2="6" y2="12" />
            <line x1="18" y1="12" x2="22" y2="12" />
            <line x1="4.93" y1="19.07" x2="7.76" y2="16.24" />
            <line x1="16.24" y1="7.76" x2="19.07" y2="4.93" />
          </svg>
        ) : (
          // Moon icon
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
          </svg>
        )}
      </button>
    </header>
  )
}
