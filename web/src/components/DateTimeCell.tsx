import { formatDateTimeParts } from '../lib/wallclock'

/**
 * Render a timestamp as localized date and time in separate spans so they sit
 * on one line when there's room and stack (date first, time second) when the
 * column is narrow. Falls back to an em dash on missing/invalid input.
 */
export function DateTimeCell({ ts }: { ts?: string }) {
  const parts = formatDateTimeParts(ts)
  if (!parts) return <>—</>
  return (
    <>
      <span className="dt-date">{parts.date}</span>{' '}
      <span className="dt-time">{parts.time}</span>
    </>
  )
}
