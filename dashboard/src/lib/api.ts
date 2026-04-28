export type DashboardLogRecord = {
  timestamp: string
  request_id: string
  model: string
  status: number
  latency_ms: number
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
  estimated_cost_eur?: number
  pii_summary?: DashboardRequestPIISummary
  security_summary?: DashboardSecuritySummary
}

export type DashboardUsageTotals = {
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  estimated_cost_eur: number
}

export type DashboardRequestPIISummary = {
  ingress_finding_count: number
  egress_finding_count: number
  entity_types: string[]
  actions: string[]
}

export type DashboardUserPIISummary = {
  flagged_request_count: number
  ingress_finding_count: number
  egress_finding_count: number
  entity_types: string[]
  last_detected_at?: string
}

export type DashboardSecuritySummary = {
  labels: string[]
}

export type DashboardPlaygroundModel = {
  provider_model_id: string
  display_name: string
  provider_type: string
  provider_display_name: string
}

export type DashboardTimeRange = {
  from?: string
  to?: string
}

export type DashboardUserLogsOverview = {
  user_id: string
  key_alias: string
  log_count: number
  logs: DashboardLogRecord[]
  time_range: {
    from?: string
    to?: string
  }
}

export type DashboardUserUsageOverview = {
  user_id: string
  key_alias: string
  usage: DashboardUsageTotals
  pii_summary?: DashboardUserPIISummary
  security_summary?: DashboardSecuritySummary
  time_range: {
    from?: string
    to?: string
  }
}

export type PlaygroundResponse = {
  proxy_status: number
  latency_ms: number
  usage?: {
    prompt_tokens?: number
    completion_tokens?: number
    total_tokens?: number
  }
  raw_json: unknown
}

export type AdminUserKey = {
  id: number
  user_id: string
  display_name: string
  key_id?: string
  masked_key: string
  status: string
  created_at: string
  revoked_at?: string
  last_used_at?: string
  monthly_cost_limit_eur: number | null
}

export type AdminProviderCredential = {
  id: number
  provider_type: string
  display_name: string
  base_url: string
  masked_key: string
  status: string
  last_validated_at?: string
  last_validation_error?: string
  created_at: string
  updated_at: string
}

export type AdminProviderModel = {
  id: number
  provider_model_id: string
  display_name: string
  enabled: boolean
  sync_state: string
  last_synced_at: string
  sheet_allowed: boolean
  sheet_last_fetched_at?: string
  metadata?: Record<string, unknown>
}

export type AdminPIIPolicy = {
  entity_type: string
  display_name: string
  enabled: boolean
  action: 'observe' | 'block'
  updated_at: string
}

export type AdminNSFWBlockedTerm = {
  id: number
  term: string
  enabled: boolean
  updated_at: string
}

export type AdminRateLimiterConfig = {
  enabled: boolean
  request_limit: number
  window_seconds: number
  quarantine_duration_seconds: number
  updated_at?: string
  updated_by?: string
}

export type AdminQuarantine = {
  user_id: string
  locked_at: string
  expires_at: string
  locked_reason: string
  unlocked_at?: string
  unlocked_by?: string
}

export class APIError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'APIError'
    this.status = status
  }
}

const apiBase = import.meta.env.VITE_API_BASE_URL ?? ''

function apiUrl(path: string): string {
  return `${apiBase}${path}`
}

async function parseAPIResponse<T>(response: Response, fallbackMessage: string): Promise<T> {
  const payload = await response
    .json()
    .catch(() => ({ error: fallbackMessage })) as T & { error?: string }

  if (!response.ok) {
    throw new APIError(response.status, payload.error ?? fallbackMessage)
  }
  return payload
}

function buildDashboardQuery(range?: DashboardTimeRange): string {
  const query = new URLSearchParams()
  if (range?.from) query.set('from', range.from)
  if (range?.to) query.set('to', range.to)
  return query.toString()
}

// ── Dashboard APIs ──

export async function fetchDashboardLogsOverview(
  range?: DashboardTimeRange,
): Promise<{ users: DashboardUserLogsOverview[] }> {
  const query = buildDashboardQuery(range)
  const response = await fetch(apiUrl(`/internal/dashboard/overview/logs${query ? `?${query}` : ''}`))
  return parseAPIResponse(response, `Failed to fetch logs overview (${response.status})`)
}

export async function fetchDashboardUsageOverview(
  range?: DashboardTimeRange,
): Promise<{ users: DashboardUserUsageOverview[] }> {
  const query = buildDashboardQuery(range)
  const response = await fetch(apiUrl(`/internal/dashboard/overview/usage${query ? `?${query}` : ''}`))
  return parseAPIResponse(response, `Failed to fetch usage overview (${response.status})`)
}

export async function fetchDashboardPlaygroundModels(): Promise<{ models: DashboardPlaygroundModel[] }> {
  const response = await fetch(apiUrl('/internal/dashboard/playground/models'))
  return parseAPIResponse(response, `Failed to fetch playground models (${response.status})`)
}

export async function submitPlaygroundRequest(input: {
  userId: string
  model: string
  prompt: string
}): Promise<{ httpStatus: number; payload: PlaygroundResponse | { error: string } }> {
  const response = await fetch(apiUrl('/internal/dashboard/playground'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: input.userId,
      model: input.model,
      prompt: input.prompt,
    }),
  })

  let payload: PlaygroundResponse | { error: string } = { error: 'Invalid response' }
  try {
    payload = await response.json()
  } catch {
    payload = { error: 'Invalid JSON response' }
  }

  return {
    httpStatus: response.status,
    payload,
  }
}

// ── Admin APIs ──

export async function fetchAdminUserKeys(): Promise<{ user_keys: AdminUserKey[] }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/tenant-keys'))
  return parseAPIResponse(response, 'Failed to fetch user keys')
}

export async function createAdminUserKey(
  input: { displayName: string },
): Promise<{ user_key: AdminUserKey; raw_api_key: string }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/tenant-keys'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ display_name: input.displayName }),
  })
  return parseAPIResponse(response, 'Failed to create user key')
}

export async function revokeAdminUserKey(id: number): Promise<void> {
  const response = await fetch(
    apiUrl(`/internal/admin/api-management/tenant-keys/${id}/revoke`),
    { method: 'POST' },
  )
  await parseAPIResponse(response, 'Failed to revoke user key')
}

export async function deleteAdminUserKey(id: number): Promise<void> {
  const response = await fetch(apiUrl(`/internal/admin/api-management/tenant-keys/${id}`), {
    method: 'DELETE',
  })
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({ error: 'Failed to delete user key' }))) as { error?: string }
    throw new APIError(response.status, payload.error ?? 'Failed to delete user key')
  }
}

export async function setUserKeyCostLimit(id: number, limitEUR: number | null): Promise<void> {
  const response = await fetch(apiUrl(`/internal/admin/api-management/tenant-keys/${id}`), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ monthly_cost_limit_eur: limitEUR }),
  })
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({ error: 'Failed to update cost limit' }))) as { error?: string }
    throw new APIError(response.status, payload.error ?? 'Failed to update cost limit')
  }
}

export async function fetchAdminProviders(): Promise<{ providers: AdminProviderCredential[] }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/providers'))
  return parseAPIResponse(response, 'Failed to fetch providers')
}

export async function createAdminProvider(
  input: { providerType: string; displayName: string; apiKey: string; baseURL?: string },
): Promise<{ provider: AdminProviderCredential; models: AdminProviderModel[] }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/providers'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      provider_type: input.providerType,
      display_name: input.displayName,
      api_key: input.apiKey,
      base_url: input.baseURL,
    }),
  })
  return parseAPIResponse(response, 'Failed to create provider')
}

export async function rotateAdminProvider(
  id: number,
  apiKey: string,
): Promise<{ provider: AdminProviderCredential; models: AdminProviderModel[] }> {
  const response = await fetch(apiUrl(`/internal/admin/api-management/providers/${id}/rotate`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ api_key: apiKey }),
  })
  return parseAPIResponse(response, 'Failed to rotate provider')
}

export async function deleteAdminProvider(id: number): Promise<void> {
  const response = await fetch(apiUrl(`/internal/admin/api-management/providers/${id}`), {
    method: 'DELETE',
  })
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({ error: 'Failed to delete provider' }))) as { error?: string }
    throw new APIError(response.status, payload.error ?? 'Failed to delete provider')
  }
}

export async function fetchAdminProviderModels(
  id: number,
): Promise<{ provider: AdminProviderCredential; models: AdminProviderModel[] }> {
  const response = await fetch(apiUrl(`/internal/admin/api-management/providers/${id}/models`))
  return parseAPIResponse(response, 'Failed to fetch provider models')
}

export async function refreshAdminProviderModels(
  id: number,
): Promise<{ provider: AdminProviderCredential; models: AdminProviderModel[] }> {
  const response = await fetch(
    apiUrl(`/internal/admin/api-management/providers/${id}/models/refresh`),
    { method: 'POST' },
  )
  return parseAPIResponse(response, 'Failed to refresh provider models')
}

export async function updateAdminProviderModels(
  id: number,
  enabledModelIDs: string[],
): Promise<{ provider: AdminProviderCredential; models: AdminProviderModel[] }> {
  const response = await fetch(apiUrl(`/internal/admin/api-management/providers/${id}/models`), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled_model_ids: enabledModelIDs }),
  })
  return parseAPIResponse(response, 'Failed to update provider models')
}

export async function fetchAdminPIIPolicies(): Promise<{ policies: AdminPIIPolicy[] }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/pii-policy'))
  return parseAPIResponse(response, 'Failed to fetch PII policy')
}

export async function updateAdminPIIPolicies(
  policies: Array<Pick<AdminPIIPolicy, 'entity_type' | 'enabled' | 'action'>>,
): Promise<{ policies: AdminPIIPolicy[] }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/pii-policy'), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ policies }),
  })
  return parseAPIResponse(response, 'Failed to update PII policy')
}

export async function fetchAdminNSFWTerms(): Promise<{ terms: AdminNSFWBlockedTerm[] }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/nsfw-terms'))
  return parseAPIResponse(response, 'Failed to fetch NSFW terms')
}

export async function createAdminNSFWTerm(
  input: Pick<AdminNSFWBlockedTerm, 'term' | 'enabled'>,
): Promise<{ term: AdminNSFWBlockedTerm }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/nsfw-terms'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ term: input.term, enabled: input.enabled }),
  })
  return parseAPIResponse(response, 'Failed to create NSFW term')
}

export async function updateAdminNSFWTerm(
  id: number,
  input: Pick<AdminNSFWBlockedTerm, 'term' | 'enabled'>,
): Promise<{ term: AdminNSFWBlockedTerm }> {
  const response = await fetch(apiUrl(`/internal/admin/api-management/nsfw-terms/${id}`), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ term: input.term, enabled: input.enabled }),
  })
  return parseAPIResponse(response, 'Failed to update NSFW term')
}

export async function deleteAdminNSFWTerm(id: number): Promise<void> {
  const response = await fetch(apiUrl(`/internal/admin/api-management/nsfw-terms/${id}`), {
    method: 'DELETE',
  })
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({ error: 'Failed to delete NSFW term' }))) as { error?: string }
    throw new APIError(response.status, payload.error ?? 'Failed to delete NSFW term')
  }
}

export async function getRateLimiterConfig(): Promise<{ config: AdminRateLimiterConfig }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/guardrails/rate-limiter'))
  return parseAPIResponse(response, 'Failed to fetch rate limiter config')
}

export async function saveRateLimiterConfig(
  config: Pick<AdminRateLimiterConfig, 'enabled' | 'request_limit' | 'window_seconds' | 'quarantine_duration_seconds'>,
): Promise<{ config: AdminRateLimiterConfig }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/guardrails/rate-limiter'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
  return parseAPIResponse(response, 'Failed to save rate limiter config')
}

export async function listQuarantines(): Promise<{ quarantines: AdminQuarantine[] }> {
  const response = await fetch(apiUrl('/internal/admin/api-management/quarantine'))
  return parseAPIResponse(response, 'Failed to fetch quarantines')
}

export async function unlockQuarantine(userID: string): Promise<void> {
  const response = await fetch(
    apiUrl(`/internal/admin/api-management/quarantine/${encodeURIComponent(userID)}`),
    { method: 'DELETE' },
  )
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({ error: 'Failed to unlock quarantine' }))) as { error?: string }
    throw new APIError(response.status, payload.error ?? 'Failed to unlock quarantine')
  }
}
