import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect } from 'vitest'
import { SecretStorePanel } from './SecretStorePanel'

const wrap = (ui: React.ReactNode) => render(<MemoryRouter>{ui}</MemoryRouter>)

describe('SecretStorePanel', () => {
  it('shows the resolved file and the flattened key names', () => {
    wrap(<SecretStorePanel info={{
      name: 'localsecretstore', type: 'secretstores.local.file',
      file: '/tmp/secrets.json', keys: ['redis:password', 'apiKey'], usedBy: ['statestore'],
    }} />)
    expect(screen.getByText('/tmp/secrets.json')).toBeInTheDocument()
    expect(screen.getByText('redis:password')).toBeInTheDocument()
    expect(screen.getByText('statestore')).toBeInTheDocument()
  })

  it('reports an init failure instead of an empty key list', () => {
    wrap(<SecretStorePanel info={{
      name: 's', type: 'secretstores.local.file',
      file: '/tmp/missing.json', initErr: 'open /tmp/missing.json: no such file or directory',
    }} />)
    expect(screen.getByText(/no such file or directory/)).toBeInTheDocument()
  })

  it('says why keys are not listed for a prefix-less env store', () => {
    wrap(<SecretStorePanel info={{ name: 'envsecrets', type: 'secretstores.local.env' }} />)
    expect(screen.getByText(/set a prefix/i)).toBeInTheDocument()
  })

  it('lists env keys when a prefix is set', () => {
    wrap(<SecretStorePanel info={{
      name: 'envsecrets', type: 'secretstores.local.env', prefix: 'MYAPP_', keys: ['ONE'],
    }} />)
    expect(screen.getByText('ONE')).toBeInTheDocument()
    expect(screen.getByText(/MYAPP_/)).toBeInTheDocument()
  })

  it('states the cap rather than truncating silently', () => {
    wrap(<SecretStorePanel info={{
      name: 's', type: 'secretstores.local.file', file: '/tmp/s.json',
      keys: ['a'], keysCapped: true,
    }} />)
    expect(screen.getByText(/more/i)).toBeInTheDocument()
  })
})
