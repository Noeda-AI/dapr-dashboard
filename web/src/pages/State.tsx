import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useStateStores } from '../hooks/useWorkflows'
import { useStateAppIds, useStateRecord, useStateRecords } from '../hooks/useStateRecords'
import { useDocumentTitle } from '../lib/useDocumentTitle'
import { DateTimeCell } from '../components/DateTimeCell'
import { dedupeStores } from '../lib/dedupeStores'
import { highlightJson } from '../lib/json-highlight'
import type { StateStore } from '../types/workflow'
import type { StateItem } from '../types/state'

const STORE_KEY = 'devdash.stateStore'

/** Humanize a byte count for the Size column. */
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const kb = bytes / 1024
  if (kb < 1024) return `${kb.toFixed(1)} KB`
  return `${(kb / 1024).toFixed(1)} MB`
}

/**
 * The expanded row's body: the full value plus the metadata the table
 * abbreviates. The value is fetched only while this is mounted.
 */
function RecordPanel({ recordKey, store }: { recordKey: string; store?: string }) {
  const { data, isLoading, isError } = useStateRecord(recordKey, store)

  if (isLoading) {
    return (
      <p className="muted" style={{ padding: 12 }}>
        Loading…
      </p>
    )
  }
  if (isError || !data) {
    return (
      <p className="muted" style={{ padding: 12 }}>
        Couldn't load this value.
      </p>
    )
  }
  return (
    <div data-testid="record-panel" style={{ padding: 12 }}>
      <div className="muted mono" style={{ marginBottom: 8, wordBreak: 'break-all' }}>
        {data.key}
      </div>
      <pre className="json" style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
        {highlightJson(data.value)}
      </pre>
      <div className="muted" style={{ marginTop: 8, fontSize: 12 }}>
        {formatSize(data.size)} · version {data.etag || '—'} · {data.encoding}
        {data.contentType ? ` · ${data.contentType}` : ''}
        {data.truncated ? ' · truncated at 1 MB' : ''}
      </div>
      <button
        type="button"
        className="btn ghost"
        style={{ marginTop: 8 }}
        onClick={() => void navigator.clipboard?.writeText(data.value)}
      >
        Copy value
      </button>
    </div>
  )
}

export function State() {
  const [searchParams, setSearchParams] = useSearchParams()
  useDocumentTitle('State')

  const urlApp = searchParams.get('app') ?? ''
  const urlSearch = searchParams.get('search') ?? ''
  const urlPage = searchParams.get('page') ?? undefined

  const [selectedApp, setSelectedApp] = useState(urlApp)
  const [searchInput, setSearchInput] = useState(urlSearch)
  const [debouncedSearch, setDebouncedSearch] = useState(urlSearch)
  const [includeInternal, setIncludeInternal] = useState(false)
  const [page, setPage] = useState<string | undefined>(urlPage)
  // The API returns only a forward cursor, so Prev is served by stacking the
  // (token, offset) of each page we leave. Empty = on the first page.
  const [history, setHistory] = useState<{ token: string | undefined; offset: number }[]>([])
  const [pageOffset, setPageOffset] = useState(0)
  // Full key of the expanded row, or null. One row at a time: the panel is
  // tall, and two open panels make the table unreadable.
  const [expandedKey, setExpandedKey] = useState<string | null>(null)

  function resetPaging() {
    setPage(undefined)
    setHistory([])
    setPageOffset(0)
    // The expanded row may not exist under the new filter/store/page.
    setExpandedKey(null)
  }

  // Stores. The dropdown collapses entries that differ only by file path, since
  // they read identical data; the choice is a store id, persisted across reloads.
  const { data: storeList } = useStateStores()
  const storesResolved = storeList !== undefined
  const noStores = storesResolved && storeList.length === 0
  const activeStore = storeList?.find((s) => s.active) ?? storeList?.[0]
  const displayStores = useMemo(() => dedupeStores(storeList ?? []), [storeList])

  // null = not yet determined; the list query stays disabled until it resolves,
  // which avoids a double-fetch on mount.
  const [selectedStore, setSelectedStore] = useState<string | null>(null)
  useEffect(() => {
    if (!displayStores || displayStores.length === 0) return
    if (selectedStore !== null && displayStores.some((s) => s.id === selectedStore)) return
    const persisted = window.localStorage.getItem(STORE_KEY)
    const fromPersisted =
      persisted && displayStores.some((s) => s.id === persisted) ? persisted : undefined
    setSelectedStore(fromPersisted ?? activeStore?.id ?? displayStores[0].id)
  }, [displayStores, activeStore, selectedStore])

  const selectedStoreObj = useMemo(
    () => storeList?.find((s) => s.id === selectedStore),
    [storeList, selectedStore],
  )

  function storeOptionLabel(s: StateStore): string {
    const typeShort = s.type.split('.').pop() ?? s.type
    const head = `${s.name} — ${s.connection ? `${typeShort} · ${s.connection}` : typeShort}`
    return s.active ? `${head} (active)` : head
  }

  function onStoreChange(id: string) {
    setSelectedStore(id)
    window.localStorage.setItem(STORE_KEY, id)
    // A different store has different prefixes — reset the app filter.
    setSelectedApp('')
    resetPaging()
  }

  // Debounce search ~250ms so typing does not fire a request per keystroke.
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    debounceTimer.current = setTimeout(() => setDebouncedSearch(searchInput), 250)
    return () => {
      if (debounceTimer.current) clearTimeout(debounceTimer.current)
    }
  }, [searchInput])

  useEffect(() => {
    const params: Record<string, string> = {}
    if (selectedApp) params.app = selectedApp
    if (debouncedSearch) params.search = debouncedSearch
    if (page) params.page = page
    setSearchParams(params, { replace: true })
  }, [selectedApp, debouncedSearch, page, setSearchParams])

  const { data, isLoading, isError, error } = useStateRecords({
    appId: selectedApp || undefined,
    search: debouncedSearch || undefined,
    page,
    store: selectedStore ?? undefined,
    includeInternal,
    enabled: selectedStore !== null,
  })

  const { data: storeAppIds } = useStateAppIds({
    store: selectedStore ?? undefined,
    enabled: selectedStore !== null,
  })
  const appIds = useMemo(() => storeAppIds ?? [], [storeAppIds])

  // On error, treat the page as empty so the pager reads "No results" and no
  // row-derived UI acts on stale data TanStack Query may have retained.
  const items = useMemo<StateItem[]>(
    () => (isError ? [] : (data?.items ?? [])),
    [isError, data?.items],
  )

  if (noStores) {
    return (
      <div className="page">
        <p className="err b">No state store detected</p>
        <p className="muted" style={{ marginTop: 8 }}>
          Configure one with the <span className="mono">--statestore</span> flag or add a state
          store component.
        </p>
      </div>
    )
  }

  // Any load error degrades gracefully: the chrome stays usable so the user can
  // switch to a reachable store.
  let loadError: string | null = null
  if (isError) {
    const errStr = String(error)
    if (errStr.includes('503')) {
      const extracted = errStr
        .replace(/^.*?503[:\s]+/, '')
        .replace(/\s*for\s+\/\S*$/, '')
        .trim()
      loadError = extracted && extracted !== errStr ? extracted : 'state store unavailable'
    } else {
      loadError = `Error loading state records: ${errStr}`
    }
  }

  return (
    <div className="page">
      <div className="phead">
        <div>
          <h1>State records</h1>
          <div className="sub">
            {appIds.length > 0
              ? `Across ${appIds.length} prefix${appIds.length !== 1 ? 'es' : ''} · read only`
              : 'Read only'}
          </div>
        </div>
        <div className="ctrlset">
          {storeList && storeList.length > 0 ? (
            <>
              <span className="led" />
              <select
                className="select"
                data-testid="store-select"
                aria-label="Switch state store"
                value={selectedStore ?? ''}
                onChange={(e) => onStoreChange(e.target.value)}
              >
                {displayStores.map((s) => (
                  <option key={s.id} value={s.id}>
                    {storeOptionLabel(s)}
                  </option>
                ))}
              </select>
              {selectedStoreObj && (
                <Link
                  className="chip"
                  to={`/components/${selectedStoreObj.name}`}
                  aria-label={`Open the ${selectedStoreObj.name} component page`}
                  title={`Open the ${selectedStoreObj.name} component page`}
                >
                  component
                </Link>
              )}
            </>
          ) : (
            <span className="chip">
              <span className="led" />
              statestore <b>unknown</b>
            </span>
          )}
        </div>
      </div>

      {loadError && (
        <div
          data-testid="load-error-banner"
          style={{
            marginBottom: 12,
            padding: '8px 12px',
            borderRadius: 8,
            border: '1px solid var(--line)',
            background: 'var(--surface)',
            color: 'var(--fail-fg)',
            fontSize: 13,
          }}
        >
          {loadError} — Select another state store or check the connection.
        </div>
      )}

      <div className="filters">
        <select
          className="select"
          data-testid="app-select"
          aria-label="Filter by app"
          value={selectedApp}
          onChange={(e) => {
            setSelectedApp(e.target.value)
            resetPaging()
          }}
        >
          <option value="">All apps</option>
          {selectedApp && !appIds.includes(selectedApp) && (
            <option value={selectedApp}>{selectedApp}</option>
          )}
          {appIds.map((id) => (
            <option key={id} value={id}>
              {id}
            </option>
          ))}
        </select>

        <label className="search">
          🔍
          <input
            placeholder="Search key…"
            aria-label="Search key"
            value={searchInput}
            onChange={(e) => {
              setSearchInput(e.target.value)
              resetPaging()
            }}
          />
        </label>

        <label className="childtoggle">
          <input
            type="checkbox"
            aria-label="Show internal keys"
            checked={includeInternal}
            onChange={(e) => {
              setIncludeInternal(e.target.checked)
              resetPaging()
            }}
          />
          Show internal keys
        </label>
      </div>

      <div className="card">
        <div className="tablewrap">
          {isLoading || (!noStores && selectedStore === null) ? (
            <p className="muted" style={{ padding: 20 }}>
              Loading…
            </p>
          ) : isError ? (
            <p className="muted" style={{ padding: 20 }}>
              Couldn't load state records from this store.
            </p>
          ) : items.length === 0 ? (
            <p className="muted" style={{ padding: 20 }}>
              No state records found
            </p>
          ) : (
            <table className="wf">
              <thead>
                <tr>
                  <th>Key</th>
                  <th>App</th>
                  <th>Value</th>
                  <th>Size</th>
                  <th>Version</th>
                  <th>TTL</th>
                </tr>
              </thead>
              <tbody>
                {items.map((rec) => {
                  const expanded = expandedKey === rec.key
                  return (
                    <Fragment key={rec.key}>
                      <tr
                        className={expanded ? 'sel' : undefined}
                        onClick={() => setExpandedKey(expanded ? null : rec.key)}
                      >
                        <td className="iid mono">
                          <span aria-hidden="true" style={{ marginRight: 6 }}>
                            {expanded ? '▾' : '▸'}
                          </span>
                          {rec.logicalKey}
                          {rec.kind !== 'app' && (
                            <span className="typechip" style={{ marginLeft: 6 }}>
                              {rec.kind}
                            </span>
                          )}
                        </td>
                        <td>{rec.appId || '—'}</td>
                        <td className="mono">
                          {rec.preview}
                          {rec.encoding === 'base64' && (
                            <span className="typechip" style={{ marginLeft: 6 }}>
                              base64
                            </span>
                          )}
                        </td>
                        <td className="mono tabnum">{formatSize(rec.size)}</td>
                        <td
                          className="mono tabnum"
                          title="Backend revision counter (etag) — it changes on every write, but is not a timestamp"
                        >
                          {rec.etag || '—'}
                        </td>
                        <td className="muted mono tabnum dt">
                          <DateTimeCell ts={rec.ttlExpiresAt} />
                        </td>
                      </tr>
                      {expanded && (
                        <tr>
                          <td colSpan={6} style={{ background: 'var(--surface)' }}>
                            <RecordPanel recordKey={rec.key} store={selectedStore ?? undefined} />
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>

        <div className="pager">
          <span className="mono">
            {items.length > 0
              ? `${pageOffset + 1}–${pageOffset + items.length} loaded`
              : 'No results'}
          </span>
          <div className="pgbtns">
            <button
              disabled={history.length === 0}
              onClick={() => {
                if (history.length === 0) return
                const prev = history[history.length - 1]
                setPage(prev.token)
                setPageOffset(prev.offset)
                setHistory((h) => h.slice(0, -1))
                setExpandedKey(null)
              }}
            >
              ← Prev
            </button>
            <button
              disabled={isError || !data?.nextToken}
              onClick={() => {
                if (!data?.nextToken) return
                setHistory((h) => [...h, { token: page, offset: pageOffset }])
                setPageOffset((o) => o + items.length)
                setPage(data.nextToken)
                setExpandedKey(null)
              }}
            >
              Next →
            </button>
          </div>
        </div>
      </div>

      <p className="hint">
        Tip — records are read only. Use “Show internal keys” to reveal workflow history and actor
        state.
      </p>
    </div>
  )
}
