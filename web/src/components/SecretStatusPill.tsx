import type { SecretStatus } from '../types/resources'

// Class tokens are prefixed with the component's own name so they cannot
// collide with unrelated global rules — see web/STYLEGUIDE.md.
const STATUS_CLASS: Record<SecretStatus, string> = {
  resolved: 'secref-ok',
  'store-not-specified': 'secref-err',
  'store-not-found': 'secref-err',
  'store-unsupported': 'secref-warn',
  'store-unreadable': 'secref-err',
  'key-not-found': 'secref-err',
  'empty-value': 'secref-warn',
  forbidden: 'secref-warn',
}

export function SecretStatusPill({ status }: { status: SecretStatus }) {
  const cls = STATUS_CLASS[status] ?? 'secref-warn'
  return (
    <span data-cy="secret-status-pill" className={'pill ' + cls}>
      {status.replace(/-/g, ' ').toUpperCase()}
    </span>
  )
}
