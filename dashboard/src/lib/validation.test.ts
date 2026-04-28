import { describe, expect, it } from 'vitest'
import { validateUserID } from './validation'

describe('validateUserID', () => {
  it('returns error for empty user id', () => {
    expect(validateUserID('   ')).toBe('User ID is required.')
  })

  it('returns null for non-empty user id', () => {
    expect(validateUserID('user-123')).toBeNull()
  })
})
