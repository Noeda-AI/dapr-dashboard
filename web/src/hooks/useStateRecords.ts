import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiUrl, fetchJSON } from '../lib/api'
import { useRefreshInterval, refetchMs } from '../lib/refresh'
import type { StateDeleteResult, StateListResult, StateRecord } from '../types/state'

interface StateRecordsParams {
  appId?: string
  search?: string
  page?: string
  limit?: number
  store?: string
  includeInternal?: boolean
  enabled?: boolean
}

function queryString(p: StateRecordsParams): string {
  const sp = new URLSearchParams()
  if (p.appId) sp.set('appId', p.appId)
  if (p.search) sp.set('search', p.search)
  if (p.page) sp.set('page', p.page)
  if (p.limit) sp.set('limit', String(p.limit))
  if (p.store) sp.set('store', p.store)
  if (p.includeInternal) sp.set('includeInternal', 'true')
  const s = sp.toString()
  return s ? `?${s}` : ''
}

export function useStateRecords(params: StateRecordsParams) {
  const ctx = useRefreshInterval()
  const qs = queryString(params)
  return useQuery<StateListResult>({
    queryKey: ['state-records', qs],
    queryFn: () => fetchJSON<StateListResult>(`/state${qs}`),
    refetchInterval: refetchMs(ctx),
    enabled: params.enabled !== false,
  })
}

/**
 * One record's full value. Enabled only while its row is expanded, so a wide
 * table never pulls megabytes of values it will not show. Not on the refresh
 * interval: an expanded value is a snapshot the user is reading.
 */
export function useStateRecord(key: string, store?: string, enabled = true) {
  const sp = new URLSearchParams({ key })
  if (store) sp.set('store', store)
  return useQuery<StateRecord>({
    queryKey: ['state-record', key, store],
    queryFn: () => fetchJSON<StateRecord>(`/state/record?${sp.toString()}`),
    enabled: enabled && !!key,
  })
}

export function useStateAppIds(params: { store?: string; enabled?: boolean }) {
  const ctx = useRefreshInterval()
  const qs = params.store ? `?store=${encodeURIComponent(params.store)}` : ''
  return useQuery<string[]>({
    queryKey: ['state-appids', qs],
    queryFn: () => fetchJSON<string[]>(`/state/appids${qs}`),
    refetchInterval: refetchMs(ctx),
    enabled: params.enabled !== false,
  })
}

async function postDelete(vars: { keys: string[]; store?: string }): Promise<StateDeleteResult[]> {
  const qs = vars.store ? `?store=${encodeURIComponent(vars.store)}` : ''
  const res = await fetch(apiUrl(`/state/delete${qs}`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ keys: vars.keys }),
  })
  if (!res.ok) throw new Error(`delete failed: ${res.status}`)
  return res.json() as Promise<StateDeleteResult[]>
}

export function useDeleteStateRecords() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: postDelete,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['state-records'] })
      qc.invalidateQueries({ queryKey: ['state-record'] })
      // The app dropdown derives from the same keyspace, so it goes stale too;
      // with auto-refresh paused it would never catch up on its own.
      qc.invalidateQueries({ queryKey: ['state-appids'] })
    },
  })
}
