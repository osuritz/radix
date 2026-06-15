import type { ReactNode } from 'react'

interface KpiCardProps {
  label: string;
  value: ReactNode;
  subValue?: ReactNode;
  accent?: 'blue' | 'green' | 'yellow' | 'red' | 'lavender';
}

const accentMap: Record<string, string> = {
  blue: 'var(--ctp-blue)',
  green: 'var(--ctp-green)',
  yellow: 'var(--ctp-yellow)',
  red: 'var(--ctp-red)',
  lavender: 'var(--ctp-lavender)',
}

export function KpiCard({ label, value, subValue, accent = 'blue' }: KpiCardProps) {
  const accentColor = accentMap[accent] ?? accentMap['blue']

  return (
    <div
      style={{
        backgroundColor: 'var(--ctp-surface)',
        borderColor: 'var(--ctp-border)',
        borderTopColor: accentColor,
      }}
      className="rounded-lg border border-t-2 p-4 flex flex-col gap-1 min-w-0"
    >
      <span style={{ color: 'var(--ctp-subtext)' }} className="text-xs font-medium uppercase tracking-wider">
        {label}
      </span>
      <span style={{ color: 'var(--ctp-text)' }} className="text-2xl font-bold tabular-nums leading-tight truncate">
        {value}
      </span>
      {subValue && (
        <span style={{ color: 'var(--ctp-overlay)' }} className="text-xs">
          {subValue}
        </span>
      )}
    </div>
  )
}
