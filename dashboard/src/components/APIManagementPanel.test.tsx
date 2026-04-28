import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import PIIPolicySection from './PIIPolicySection'

describe('APIManagementPanel', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('loads and updates the active pii policy', async () => {
    const onSave = vi.fn()
    const onUpdate = vi.fn()

    render(
      <PIIPolicySection
        piiPolicies={[
          {
            entity_type: 'email_address',
            display_name: 'Email Address',
            enabled: true,
            action: 'observe',
            updated_at: '2026-03-17T10:00:00Z',
          },
        ]}
        policyStatus="idle"
        onSave={onSave}
        onUpdate={onUpdate}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('Email Address')).toBeTruthy()
    })

    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.change(screen.getByRole('combobox'), {
      target: { value: 'block' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save PII Policy' }))

    expect(onUpdate).toHaveBeenCalledWith('email_address', { enabled: false })
    expect(onUpdate).toHaveBeenCalledWith('email_address', { action: 'block' })
    expect(onSave).toHaveBeenCalledTimes(1)
  })
})
