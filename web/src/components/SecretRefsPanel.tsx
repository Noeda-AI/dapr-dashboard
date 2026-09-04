import { Fragment } from 'react'
import { SecretStatusPill } from './SecretStatusPill'
import { useSecretReveal } from '../hooks/useSecretReveal'
import { getCapabilities } from '../lib/capabilities'
import { trackAction } from '../lib/telemetry'
import type { SecretRefStatus } from '../types/resources'

interface Props {
  resourceId: string
  refs: SecretRefStatus[]
  /** Component type, for telemetry. Never the name — that can be identifying. */
  componentType?: string
}

export function SecretRefsPanel({ resourceId, refs, componentType }: Props) {
  const { values, reveal, hide } = useSecretReveal(resourceId)
  const revealAllowed = getCapabilities().secretReveal !== false

  return (
    <div className="panel" style={{ marginBottom: 12 }}>
      <div className="ph">Secret references</div>
      <div className="kv">
        {refs.map((r) => {
          const shown = values[r.field]
          const isResolved = r.status === 'resolved'
          return (
            <Fragment key={r.field}>
              <div className="kk">{r.field}</div>
              <div className="vv" style={{ flexWrap: 'wrap', gap: 8 }}>
                <SecretStatusPill status={r.status} />
                <span className="mono" style={{ fontSize: 12 }}>
                  {r.kind === 'envRef' ? `env ${r.name}` : `${r.store ?? '—'} → ${r.key || r.name}`}
                </span>
                {isResolved && (
                  <>
                    <span className="mono" style={{ fontSize: 12 }}>{shown ?? '••••••••'}</span>
                    <button
                      className="btn"
                      aria-label={shown ? `hide ${r.field}` : `reveal ${r.field}`}
                      aria-pressed={!!shown}
                      disabled={!revealAllowed}
                      title={revealAllowed ? undefined : 'Reveal is unavailable when the dashboard is served off-host'}
                      onClick={() => {
                        if (shown) return hide(r.field)
                        trackAction('secret_reveal', { componentType: componentType ?? '' })
                        void reveal(r.field)
                      }}
                    >
                      {shown ? 'Hide' : 'Reveal'}
                    </button>
                  </>
                )}
                {!isResolved && r.detail && (
                  <span style={{ fontSize: 12, color: 'var(--muted)' }}>{r.detail}</span>
                )}
              </div>
            </Fragment>
          )
        })}
      </div>
    </div>
  )
}
