export function validateUserID(userID: string): string | null {
  if (!userID.trim()) {
    return 'User ID is required.'
  }
  return null
}
