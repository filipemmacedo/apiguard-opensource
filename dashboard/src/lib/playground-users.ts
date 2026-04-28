import type { AdminUserKey } from './api'

export type PlaygroundUserOption = {
  value: string
  label: string
  description?: string
}

export function buildPlaygroundUserOptions(userKeys: AdminUserKey[]): PlaygroundUserOption[] {
  const activeUsers = new Map<string, { label: string; activeKeyCount: number }>()

  for (const userKey of userKeys) {
    if (userKey.status !== 'active') {
      continue
    }

    const userID = userKey.user_id.trim()
    if (!userID) {
      continue
    }

    const displayName = userKey.display_name.trim() || userID
    const existing = activeUsers.get(userID)
    if (existing) {
      existing.activeKeyCount += 1
      if (existing.label === userID && displayName !== userID) {
        existing.label = displayName
      }
      continue
    }

    activeUsers.set(userID, {
      label: displayName,
      activeKeyCount: 1,
    })
  }

  return Array.from(activeUsers.entries())
    .map(([userID, user]) => {
      const details = []
      if (user.label !== userID) {
        details.push(userID)
      }
      details.push(`${user.activeKeyCount} active ${user.activeKeyCount === 1 ? 'key' : 'keys'}`)

      return {
        value: userID,
        label: user.label,
        description: details.join(' | '),
      }
    })
    .sort((left, right) => left.label.localeCompare(right.label))
}
