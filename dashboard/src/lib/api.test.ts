import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createAdminProvider,
  createAdminUserKey,
  createAdminNSFWTerm,
  deleteAdminNSFWTerm,
  fetchAdminNSFWTerms,
  fetchAdminPIIPolicies,
  fetchAdminProviderModels,
  fetchAdminUserKeys,
  fetchDashboardLogs,
  fetchDashboardLogsOverview,
  fetchDashboardPlaygroundModels,
  fetchDashboardUsage,
  fetchDashboardUsageOverview,
  updateAdminNSFWTerm,
  updateAdminPIIPolicies,
  updateAdminProviderModels,
} from './api'

describe('dashboard API query params', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('includes from/to for logs requests when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ user_id: 'user-a', logs: [] }),
    })

    await fetchDashboardLogs(fetchMock, {
      from: '2026-03-12T10:00:00Z',
      to: '2026-03-12T11:00:00Z',
    })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const url = String(fetchMock.mock.calls[0]?.[0])
    expect(url).toContain('/internal/dashboard/logs?')
    expect(url).toContain('from=2026-03-12T10%3A00%3A00Z')
    expect(url).toContain('to=2026-03-12T11%3A00%3A00Z')
  })

  it('includes from/to for usage requests when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        user_id: 'user-a',
        usage: { prompt_tokens: 0, completion_tokens: 0, total_tokens: 0 },
      }),
    })

    await fetchDashboardUsage(fetchMock, {
      from: '2026-03-12T10:00:00Z',
      to: '2026-03-12T11:00:00Z',
    })

    const url = String(fetchMock.mock.calls[0]?.[0])
    expect(url).toContain('/internal/dashboard/usage?')
    expect(url).toContain('from=2026-03-12T10%3A00%3A00Z')
    expect(url).toContain('to=2026-03-12T11%3A00%3A00Z')
  })

  it('includes from/to for logs overview requests when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ users: [] }),
    })

    await fetchDashboardLogsOverview(fetchMock, {
      from: '2026-03-12T10:00:00Z',
      to: '2026-03-12T11:00:00Z',
    })

    const url = String(fetchMock.mock.calls[0]?.[0])
    expect(url).toContain('/internal/dashboard/overview/logs?')
    expect(url).toContain('from=2026-03-12T10%3A00%3A00Z')
    expect(url).toContain('to=2026-03-12T11%3A00%3A00Z')
  })

  it('includes from/to for usage overview requests when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ users: [] }),
    })

    await fetchDashboardUsageOverview(fetchMock, {
      from: '2026-03-12T10:00:00Z',
      to: '2026-03-12T11:00:00Z',
    })

    const url = String(fetchMock.mock.calls[0]?.[0])
    expect(url).toContain('/internal/dashboard/overview/usage?')
    expect(url).toContain('from=2026-03-12T10%3A00%3A00Z')
    expect(url).toContain('to=2026-03-12T11%3A00%3A00Z')
  })

  it('loads the playground model catalog from the dashboard endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ models: [] }),
    })

    await fetchDashboardPlaygroundModels(fetchMock)

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/dashboard/playground/models')
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toBeUndefined()
  })

  it('loads user key management without a separate admin authorization header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ user_keys: [] }),
    })

    await fetchAdminUserKeys(fetchMock)

    const [, options] = fetchMock.mock.calls[0] ?? []
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('/internal/admin/api-management/tenant-keys')
    expect(options?.headers).toBeUndefined()
  })

  it('posts user key creation payload with display name only', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ user_key: { id: 1, user_id: 'user-123', display_name: 'Acme' }, raw_api_key: 'agtk_abc' }),
    })

    await createAdminUserKey(fetchMock, {
      displayName: 'Acme Production',
    })

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/admin/api-management/tenant-keys')
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe('POST')
    expect(String(fetchMock.mock.calls[0]?.[1]?.body)).toContain('"display_name":"Acme Production"')
    expect(String(fetchMock.mock.calls[0]?.[1]?.body)).not.toContain('"user_id"')
  })

  it('posts provider creation payload without leaking the key in the URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ provider: {}, models: [] }),
    })

    await createAdminProvider(fetchMock, {
      providerType: 'openai',
      displayName: 'OpenAI',
      apiKey: 'sk-secret',
      baseURL: 'https://api.openai.com',
    })

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/admin/api-management/providers')
    const [, options] = fetchMock.mock.calls[0] ?? []
    expect(options?.method).toBe('POST')
    expect(String(options?.body)).toContain('"api_key":"sk-secret"')
  })

  it('fetches provider models without a separate admin authorization header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ provider: {}, models: [] }),
    })

    await fetchAdminProviderModels(fetchMock, 42)

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      '/internal/admin/api-management/providers/42/models',
    )
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toBeUndefined()
  })

  it('patches enabled model ids for provider model updates', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ provider: {}, models: [] }),
    })

    await updateAdminProviderModels(fetchMock, 42, ['gpt-4.1', 'gpt-4o-mini'])

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      '/internal/admin/api-management/providers/42/models',
    )
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe('PATCH')
    expect(String(fetchMock.mock.calls[0]?.[1]?.body)).toContain('"enabled_model_ids"')
  })

  it('fetches pii policies without a separate admin authorization header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ policies: [] }),
    })

    await fetchAdminPIIPolicies(fetchMock)

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/admin/api-management/pii-policy')
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toBeUndefined()
  })

  it('patches pii policies in the request body', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ policies: [] }),
    })

    await updateAdminPIIPolicies(fetchMock, [
      {
        entity_type: 'email_address',
        enabled: true,
        action: 'block',
      },
    ])

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/admin/api-management/pii-policy')
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe('PATCH')
    expect(String(fetchMock.mock.calls[0]?.[1]?.body)).toContain('"entity_type":"email_address"')
    expect(String(fetchMock.mock.calls[0]?.[1]?.body)).toContain('"action":"block"')
  })

  it('fetches nsfw terms without a separate admin authorization header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ terms: [] }),
    })

    await fetchAdminNSFWTerms(fetchMock)

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/admin/api-management/nsfw-terms')
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toBeUndefined()
  })

  it('posts nsfw term creation in the request body', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ term: { id: 1, term: 'Adult Content', enabled: true } }),
    })

    await createAdminNSFWTerm(fetchMock, {
      term: 'Adult Content',
      enabled: true,
    })

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/admin/api-management/nsfw-terms')
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe('POST')
    expect(String(fetchMock.mock.calls[0]?.[1]?.body)).toContain('"term":"Adult Content"')
  })

  it('patches nsfw terms in the request body', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ term: { id: 4, term: 'Adult Phrase', enabled: false } }),
    })

    await updateAdminNSFWTerm(fetchMock, 4, {
      term: 'Adult Phrase',
      enabled: false,
    })

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/admin/api-management/nsfw-terms/4')
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe('PATCH')
    expect(String(fetchMock.mock.calls[0]?.[1]?.body)).toContain('"enabled":false')
  })

  it('deletes nsfw terms via the delete endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
    })

    await deleteAdminNSFWTerm(fetchMock, 9)

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe('/internal/admin/api-management/nsfw-terms/9')
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe('DELETE')
  })
})
