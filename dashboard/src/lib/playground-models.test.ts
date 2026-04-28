import { describe, expect, it, vi } from 'vitest'
import {
  buildPlaygroundModelOptions,
  getNextPlaygroundModelSelection,
  handlePlaygroundPromptKeyDown,
} from './playground-models'

describe('playground model helpers', () => {
  it('builds searchable model options with provider context', () => {
    const options = buildPlaygroundModelOptions([
      {
        provider_model_id: 'gpt-4.1',
        display_name: 'GPT-4.1',
        provider_type: 'openai',
        provider_display_name: 'Primary OpenAI',
      },
      {
        provider_model_id: 'claude-3-7-sonnet',
        display_name: '',
        provider_type: 'anthropic',
        provider_display_name: 'Anthropic West',
      },
    ])

    expect(options).toEqual([
      {
        value: 'claude-3-7-sonnet',
        label: 'claude-3-7-sonnet',
        description: 'Anthropic West | Anthropic',
      },
      {
        value: 'gpt-4.1',
        label: 'GPT-4.1',
        description: 'Primary OpenAI | OpenAI',
      },
    ])
  })

  it('preserves the current model selection when it is still available', () => {
    const next = getNextPlaygroundModelSelection('gpt-4.1', [
      { value: 'gpt-4.1', label: 'GPT-4.1' },
      { value: 'gpt-4o-mini', label: 'gpt-4o-mini' },
    ])

    expect(next).toBe('gpt-4.1')
  })

  it('falls back to the first available model or empty string', () => {
    expect(
      getNextPlaygroundModelSelection('missing-model', [
        { value: 'gpt-4o-mini', label: 'gpt-4o-mini' },
      ]),
    ).toBe('gpt-4o-mini')

    expect(getNextPlaygroundModelSelection('missing-model', [])).toBe('')
  })

  it('prefers gpt-4o-mini as the default selection when available', () => {
    const next = getNextPlaygroundModelSelection('missing-model', [
      { value: 'gpt-4.1', label: 'GPT-4.1' },
      { value: 'gpt-4o-mini', label: 'gpt-4o-mini' },
    ])

    expect(next).toBe('gpt-4o-mini')
  })

  it('submits on Enter only when submission is allowed', () => {
    const preventDefault = vi.fn()
    const onSubmit = vi.fn()

    handlePlaygroundPromptKeyDown(
      {
        key: 'Enter',
        shiftKey: false,
        preventDefault,
      },
      true,
      onSubmit,
    )

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(onSubmit).toHaveBeenCalledTimes(1)
  })

  it('keeps multiline input when Shift+Enter is pressed or submit is disabled', () => {
    const preventDefault = vi.fn()
    const onSubmit = vi.fn()

    handlePlaygroundPromptKeyDown(
      {
        key: 'Enter',
        shiftKey: true,
        preventDefault,
      },
      true,
      onSubmit,
    )

    handlePlaygroundPromptKeyDown(
      {
        key: 'Enter',
        shiftKey: false,
        preventDefault,
      },
      false,
      onSubmit,
    )

    expect(preventDefault).not.toHaveBeenCalled()
    expect(onSubmit).not.toHaveBeenCalled()
  })
})
