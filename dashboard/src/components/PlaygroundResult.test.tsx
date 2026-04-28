import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import PlaygroundResult from './PlaygroundResult'

describe('PlaygroundResult', () => {
  it('renders usage and raw json for successful payload', () => {
    render(
      <PlaygroundResult
        httpStatus={200}
        payload={{
          proxy_status: 200,
          latency_ms: 34,
          usage: {
            prompt_tokens: 3,
            completion_tokens: 4,
            total_tokens: 7,
          },
          raw_json: { id: 'resp_1' },
        }}
      />,
    )

    expect(screen.getByText('HTTP status:')).toBeTruthy()
    expect(screen.getByText('Proxy status:')).toBeTruthy()
    expect(screen.getByText('prompt_tokens: 3')).toBeTruthy()
    expect(screen.getByText(/resp_1/)).toBeTruthy()
  })

  it('renders error payload for non-proxy response', () => {
    render(
      <PlaygroundResult httpStatus={500} payload={{ error: 'request failed' }} />,
    )
    expect(screen.getByText('Request failed')).toBeTruthy()
    expect(screen.getByText(/request failed/)).toBeTruthy()
  })
})
