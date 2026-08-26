import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { http, HttpResponse } from 'msw'
import { describe, it, expect, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { server } from '../test/setup'
import { QueryProvider } from '../lib/query'
import { RefreshProvider } from '../lib/refresh'
import { State } from './State'

const STORES = [
  {
    id: 's1',
    name: 'statestore',
    type: 'state.redis',
    source: 'auto',
    path: '/c/s.yaml',
    active: true,
    connection: 'localhost:6379',
  },
  {
    id: 's2',
    name: 'other',
    type: 'state.sqlite',
    source: 'manual',
    path: '',
    active: false,
    connection: '/tmp/x.db',
  },
]

const ITEM = {
  key: 'myapp||order-42',
  appId: 'myapp',
  logicalKey: 'order-42',
  kind: 'app' as const,
  preview: '{ "id": 42 }',
  encoding: 'text' as const,
  size: 1284,
  etag: '3',
  ttlExpiresAt: '2026-08-26T14:02:11Z',
}

function stubApi(overrides?: { items?: unknown[]; nextToken?: string }) {
  server.use(
    http.get('/api/statestores', () => HttpResponse.json(STORES)),
    http.get('/api/state/appids', () => HttpResponse.json(['myapp', 'other-app'])),
    http.get('/api/state', () =>
      HttpResponse.json({
        items: overrides?.items ?? [ITEM],
        nextToken: overrides?.nextToken ?? '',
      }),
    ),
  )
}

function renderAt(entry = '/state') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: 0, staleTime: 0 } } })
  const router = createMemoryRouter(
    [
      { path: '/state', element: <State /> },
      { path: '/components/:name', element: <div>component page</div> },
    ],
    { initialEntries: [entry], future: { v7_relativeSplatPath: true } },
  )
  return render(
    <QueryProvider client={client}>
      <RefreshProvider>
        <RouterProvider router={router} future={{ v7_startTransition: true }} />
      </RefreshProvider>
    </QueryProvider>,
  )
}

describe('State page', () => {
  beforeEach(() => window.localStorage.clear())

  it('renders a record row with key, app, preview, size, version and TTL', async () => {
    stubApi()
    renderAt()
    const cell = await screen.findByText('order-42')
    const row = within(cell.closest('tr') as HTMLElement)
    expect(row.getByText('myapp')).toBeInTheDocument()
    expect(row.getByText('{ "id": 42 }')).toBeInTheDocument()
    expect(row.getByText('1.3 KB')).toBeInTheDocument()
    expect(row.getByText('3')).toBeInTheDocument()
  })

  it('lists the store selector and links to the selected store component', async () => {
    stubApi()
    renderAt()
    const select = await screen.findByTestId('store-select')
    expect(select).toHaveValue('s1')
    expect(within(select as HTMLElement).getAllByRole('option')).toHaveLength(2)
    expect(screen.getByRole('link', { name: /statestore component page/i })).toHaveAttribute(
      'href',
      '/components/statestore',
    )
  })

  it('sends the selected app as a query param', async () => {
    stubApi()
    let seen = ''
    server.use(
      http.get('/api/state', ({ request }) => {
        seen = new URL(request.url).search
        return HttpResponse.json({ items: [ITEM] })
      }),
    )
    renderAt()
    await screen.findByText('order-42')
    await userEvent.selectOptions(await screen.findByTestId('app-select'), 'other-app')
    await waitFor(() => expect(seen).toContain('appId=other-app'))
  })

  it('debounces the key search into the query', async () => {
    stubApi()
    let seen = ''
    server.use(
      http.get('/api/state', ({ request }) => {
        seen = new URL(request.url).search
        return HttpResponse.json({ items: [ITEM] })
      }),
    )
    renderAt()
    await screen.findByText('order-42')
    await userEvent.type(screen.getByLabelText('Search key'), 'order')
    await waitFor(() => expect(seen).toContain('search=order'), { timeout: 2000 })
  })

  it('requests internal keys only when the toggle is on', async () => {
    stubApi()
    let seen = ''
    server.use(
      http.get('/api/state', ({ request }) => {
        seen = new URL(request.url).search
        return HttpResponse.json({ items: [ITEM] })
      }),
    )
    renderAt()
    await screen.findByText('order-42')
    expect(seen).not.toContain('includeInternal')
    await userEvent.click(screen.getByLabelText('Show internal keys'))
    await waitFor(() => expect(seen).toContain('includeInternal=true'))
  })

  it('marks a non-app record with its kind', async () => {
    stubApi({
      items: [
        {
          ...ITEM,
          key: 'myapp||MyActor||a1||balance',
          logicalKey: 'MyActor||a1||balance',
          kind: 'actor',
        },
      ],
    })
    renderAt()
    expect(await screen.findByText('actor')).toBeInTheDocument()
  })

  it('shows an em dash for a record with no version or TTL', async () => {
    stubApi({ items: [{ ...ITEM, etag: undefined, ttlExpiresAt: undefined }] })
    renderAt()
    const cell = await screen.findByText('order-42')
    const row = within(cell.closest('tr') as HTMLElement)
    expect(row.getAllByText('—').length).toBeGreaterThanOrEqual(2)
  })

  it('pages forward and back with the cursor token', async () => {
    server.use(
      http.get('/api/statestores', () => HttpResponse.json(STORES)),
      http.get('/api/state/appids', () => HttpResponse.json(['myapp'])),
      http.get('/api/state', ({ request }) => {
        const page = new URL(request.url).searchParams.get('page')
        if (page === 'tok') {
          return HttpResponse.json({
            items: [{ ...ITEM, key: 'myapp||order-99', logicalKey: 'order-99' }],
          })
        }
        return HttpResponse.json({ items: [ITEM], nextToken: 'tok' })
      }),
    )
    renderAt()
    await screen.findByText('order-42')
    expect(screen.getByText('1–1 loaded')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /next/i }))
    await screen.findByText('order-99')
    expect(screen.getByText('2–2 loaded')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /prev/i }))
    await screen.findByText('order-42')
  })

  it('shows the empty state when the store has no matching records', async () => {
    stubApi({ items: [] })
    renderAt()
    expect(await screen.findByText('No state records found')).toBeInTheDocument()
    expect(screen.getByText('No results')).toBeInTheDocument()
  })

  it('keeps the chrome usable and banners a 503 so another store can be picked', async () => {
    server.use(
      http.get('/api/statestores', () => HttpResponse.json(STORES)),
      http.get('/api/state/appids', () => HttpResponse.json([])),
      http.get('/api/state', () =>
        HttpResponse.json({ error: 'this state store cannot be browsed' }, { status: 503 }),
      ),
    )
    renderAt()
    const banner = await screen.findByTestId('load-error-banner')
    expect(banner).toHaveTextContent('cannot be browsed')
    expect(screen.getByTestId('store-select')).toBeInTheDocument()
  })

  it('guides the user when no state store is configured at all', async () => {
    server.use(http.get('/api/statestores', () => HttpResponse.json([])))
    renderAt()
    expect(await screen.findByText('No state store detected')).toBeInTheDocument()
  })

  it('persists the selected store across mounts', async () => {
    stubApi()
    renderAt()
    await userEvent.selectOptions(await screen.findByTestId('store-select'), 's2')
    await waitFor(() => expect(window.localStorage.getItem('devdash.stateStore')).toBe('s2'))
  })
})

describe('State page row expansion', () => {
  beforeEach(() => window.localStorage.clear())

  const RECORD = {
    key: 'myapp||order-42',
    appId: 'myapp',
    logicalKey: 'order-42',
    kind: 'app' as const,
    value: '{"id":42,"total":19.99}',
    encoding: 'text' as const,
    size: 23,
    truncated: false,
    etag: '3',
  }

  it('fetches and shows the full value when a row is clicked', async () => {
    stubApi()
    let calls = 0
    server.use(
      http.get('/api/state/record', ({ request }) => {
        calls++
        expect(new URL(request.url).searchParams.get('key')).toBe('myapp||order-42')
        return HttpResponse.json(RECORD)
      }),
    )
    renderAt()
    await userEvent.click(await screen.findByText('order-42'))
    expect(await screen.findByTestId('record-panel')).toHaveTextContent('"total"')
    // The full key, which the table abbreviates, is shown in the panel.
    expect(screen.getByTestId('record-panel')).toHaveTextContent('myapp||order-42')
    expect(calls).toBe(1)
  })

  it('does not fetch any value before a row is expanded', async () => {
    stubApi()
    let calls = 0
    server.use(
      http.get('/api/state/record', () => {
        calls++
        return HttpResponse.json(RECORD)
      }),
    )
    renderAt()
    await screen.findByText('order-42')
    expect(calls).toBe(0)
  })

  it('collapses on a second click and expands only one row at a time', async () => {
    stubApi({
      items: [ITEM, { ...ITEM, key: 'myapp||order-43', logicalKey: 'order-43' }],
    })
    server.use(http.get('/api/state/record', () => HttpResponse.json(RECORD)))
    renderAt()

    await userEvent.click(await screen.findByText('order-42'))
    expect(await screen.findByTestId('record-panel')).toBeInTheDocument()

    await userEvent.click(screen.getByText('order-43'))
    await waitFor(() => expect(screen.getAllByTestId('record-panel')).toHaveLength(1))

    await userEvent.click(screen.getByText('order-43'))
    await waitFor(() => expect(screen.queryByTestId('record-panel')).not.toBeInTheDocument())
  })

  it('flags a truncated value', async () => {
    stubApi()
    server.use(
      http.get('/api/state/record', () =>
        HttpResponse.json({ ...RECORD, truncated: true, size: 5_000_000 }),
      ),
    )
    renderAt()
    await userEvent.click(await screen.findByText('order-42'))
    expect(await screen.findByText(/truncated/i)).toBeInTheDocument()
  })

  it('reports a value that could not be loaded without breaking the table', async () => {
    stubApi()
    server.use(
      http.get('/api/state/record', () =>
        HttpResponse.json({ error: 'record not found' }, { status: 404 }),
      ),
    )
    renderAt()
    await userEvent.click(await screen.findByText('order-42'))
    expect(await screen.findByText(/couldn't load this value/i)).toBeInTheDocument()
    expect(screen.getByText('order-42')).toBeInTheDocument()
  })

  it('collapses the expanded row when a filter changes', async () => {
    stubApi()
    server.use(http.get('/api/state/record', () => HttpResponse.json(RECORD)))
    renderAt()
    await userEvent.click(await screen.findByText('order-42'))
    expect(await screen.findByTestId('record-panel')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Show internal keys'))
    await waitFor(() => expect(screen.queryByTestId('record-panel')).not.toBeInTheDocument())
  })
})
