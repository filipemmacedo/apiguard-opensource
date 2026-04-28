import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import NSFWBlockedTermsSection from './NSFWBlockedTermsSection'

describe('NSFWBlockedTermsSection', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders blocked terms and forwards editing actions', async () => {
    const onAdd = vi.fn()
    const onSave = vi.fn()
    const onRemove = vi.fn()
    const onUpdate = vi.fn()

    render(
      <NSFWBlockedTermsSection
        terms={[
          {
            id: 1,
            term: 'Adult Content',
            enabled: true,
            persisted: true,
          },
          {
            id: 'draft-1',
            term: 'New Phrase',
            enabled: false,
            persisted: false,
          },
        ]}
        policyStatus="idle"
        onAdd={onAdd}
        onSave={onSave}
        onRemove={onRemove}
        onUpdate={onUpdate}
      />,
    )

    await waitFor(() => {
      expect(screen.getByDisplayValue('Adult Content')).toBeTruthy()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Add Term' }))
    fireEvent.change(screen.getByDisplayValue('Adult Content'), {
      target: { value: 'Adult Phrase' },
    })
    fireEvent.click(screen.getAllByRole('checkbox')[1]!)
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove' })[1]!)
    fireEvent.click(screen.getByRole('button', { name: 'Save NSFW Terms' }))

    expect(onAdd).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenCalledWith(1, { term: 'Adult Phrase' })
    expect(onUpdate).toHaveBeenCalledWith('draft-1', { enabled: true })
    expect(onRemove).toHaveBeenCalledWith('draft-1')
    expect(onSave).toHaveBeenCalledTimes(1)
  })
})
