import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { UsageSecuritySummary } from './UsageOverviewPanel'

describe('UsageOverviewPanel', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('renders tenant security labels in the usage table', async () => {
    render(
      <UsageSecuritySummary
        summary={{
          labels: ['PII', 'NSFW'],
        }}
        piiSummary={{
          flagged_request_count: 2,
          ingress_finding_count: 2,
          egress_finding_count: 1,
          entity_types: ['email_address', 'phone_number'],
        }}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('PII')).toBeTruthy()
    })
    expect(screen.getByText('in 2 / out 1')).toBeTruthy()
    expect(screen.getByText('NSFW')).toBeTruthy()
  })

  it('falls back to pii summary even when security labels are missing', async () => {
    render(
      <UsageSecuritySummary
        piiSummary={{
          flagged_request_count: 1,
          ingress_finding_count: 1,
          egress_finding_count: 0,
          entity_types: ['email_address'],
        }}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('PII')).toBeTruthy()
    })
    expect(screen.getByText('in 1 / out 0')).toBeTruthy()
  })

  it('renders a dash when no security labels apply', async () => {
    render(<UsageSecuritySummary />)

    await waitFor(() => {
      expect(screen.getByText('No breaches')).toBeTruthy()
    })
  })

  it('renders NSFW-only breaches when no pii findings are present', async () => {
    render(
      <UsageSecuritySummary
        summary={{
          labels: ['NSFW'],
        }}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('NSFW')).toBeTruthy()
    })
  })
})
