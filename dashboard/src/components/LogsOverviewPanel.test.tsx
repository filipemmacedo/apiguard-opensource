import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  buildLogsOverviewStats,
  isSecurityBlockedLog,
  LogSecuritySummary,
  LogStatusBadge,
} from './LogsOverviewPanel'

describe('LogsOverviewPanel', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('renders per-request security labels without raw values', async () => {
    render(
      <LogSecuritySummary
        summary={{
          labels: ['PII', 'NSFW'],
        }}
        piiSummary={{
          ingress_finding_count: 1,
          egress_finding_count: 0,
          entity_types: ['email_address'],
          actions: ['observe'],
        }}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('PII')).toBeTruthy()
    })
    expect(screen.getByText('in 1 / out 0')).toBeTruthy()
    expect(screen.getByText('NSFW')).toBeTruthy()
  })

  it('falls back to pii summary even when security labels are missing', async () => {
    render(
      <LogSecuritySummary
        piiSummary={{
          ingress_finding_count: 1,
          egress_finding_count: 1,
          entity_types: ['phone_number'],
          actions: ['block'],
        }}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('PII')).toBeTruthy()
    })
    expect(screen.getByText('in 1 / out 1')).toBeTruthy()
  })

  it('renders a dash when no security labels apply', async () => {
    render(<LogSecuritySummary />)

    await waitFor(() => {
      expect(screen.getByText('-')).toBeTruthy()
    })
  })

  it('treats security-blocked requests separately from error stats', () => {
    const stats = buildLogsOverviewStats([
      {
        status: 200,
        latency_ms: 12,
      },
      {
        status: 400,
        latency_ms: 4,
        security_summary: {
          labels: ['NSFW'],
        },
      },
      {
        status: 500,
        latency_ms: 20,
      },
    ])

    expect(stats.successRate).toBe('33.3%')
    expect(stats.securityBlockRate).toBe('33.3%')
    expect(stats.errorRate).toBe('33.3%')
  })

  it('classifies only client-side security policy rejections as blocked', () => {
    expect(
      isSecurityBlockedLog({
        status: 400,
        security_summary: {
          labels: ['NSFW'],
        },
      }),
    ).toBe(true)

    expect(
      isSecurityBlockedLog({
        status: 200,
        security_summary: {
          labels: ['PII'],
        },
      }),
    ).toBe(false)

    expect(
      isSecurityBlockedLog({
        status: 500,
        security_summary: {
          labels: ['NSFW'],
        },
      }),
    ).toBe(false)
  })

  it('renders Blocked status instead of a red 400 badge for security policy blocks', async () => {
    render(
      <LogStatusBadge
        log={{
          status: 400,
          security_summary: {
            labels: ['NSFW'],
          },
        }}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('Blocked')).toBeTruthy()
    })
  })
})
