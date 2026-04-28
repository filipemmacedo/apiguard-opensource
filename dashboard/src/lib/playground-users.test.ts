import { describe, expect, it } from 'vitest'
import { buildPlaygroundUserOptions } from './playground-users'

describe('buildPlaygroundUserOptions', () => {
  it('keeps only active users and prefers display names', () => {
    const options = buildPlaygroundUserOptions([
      {
        id: 1,
        user_id: 'user-b',
        display_name: 'Beta Team',
        masked_key: 'agtk_...1234',
        status: 'active',
        created_at: '2026-04-03T12:00:00Z',
      },
      {
        id: 2,
        user_id: 'user-a',
        display_name: 'user-a',
        masked_key: 'agtk_...5678',
        status: 'active',
        created_at: '2026-04-03T12:01:00Z',
      },
      {
        id: 3,
        user_id: 'user-b',
        display_name: 'user-b',
        masked_key: 'agtk_...9999',
        status: 'active',
        created_at: '2026-04-03T12:02:00Z',
      },
      {
        id: 4,
        user_id: 'user-c',
        display_name: 'Gamma Team',
        masked_key: 'agtk_...0000',
        status: 'revoked',
        created_at: '2026-04-03T12:03:00Z',
      },
    ])

    expect(options).toEqual([
      {
        value: 'user-b',
        label: 'Beta Team',
        description: 'user-b | 2 active keys',
      },
      {
        value: 'user-a',
        label: 'user-a',
        description: '1 active key',
      },
    ])
  })
})
