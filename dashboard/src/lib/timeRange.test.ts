import { describe, expect, it } from 'vitest'
import { buildDashboardTimeRange } from './timeRange'

describe('buildDashboardTimeRange', () => {
  it('returns undefined range when both values are empty', () => {
    const result = buildDashboardTimeRange('', '')
    expect(result.range).toBeUndefined()
    expect(result.error).toBeUndefined()
  })

  it('returns error when only one bound is provided', () => {
    const result = buildDashboardTimeRange('2026-03-12T10:00', '')
    expect(result.error).toBe('Both From and To must be provided.')
  })

  it('returns error when from is not earlier than to', () => {
    const result = buildDashboardTimeRange('2026-03-12T10:00', '2026-03-12T10:00')
    expect(result.error).toBe('From must be earlier than To.')
  })

  it('returns ISO8601 bounds when inputs are valid', () => {
    const result = buildDashboardTimeRange('2026-03-12T10:00', '2026-03-12T11:00')
    expect(result.error).toBeUndefined()
    expect(result.range?.from).toMatch(/^2026-03-12T10:00:00/)
    expect(result.range?.to).toMatch(/^2026-03-12T11:00:00/)
  })
})
