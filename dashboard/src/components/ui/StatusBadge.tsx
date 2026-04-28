interface StatusBadgeProps {
  status: number | string
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const code = typeof status === 'string' ? parseInt(status, 10) : status
  const isSuccess = code >= 200 && code < 300
  const isRedirect = code >= 300 && code < 400
  const isClientError = code >= 400 && code < 500
  const isServerError = code >= 500

  let bg = '#F3F4F6'
  let color = '#6B7280'

  if (isSuccess) {
    bg = '#ECFDF5'
    color = '#059669'
  } else if (isRedirect) {
    bg = '#FFF7ED'
    color = '#D97706'
  } else if (isClientError) {
    bg = '#FEF2F2'
    color = '#DC2626'
  } else if (isServerError) {
    bg = '#FEF2F2'
    color = '#DC2626'
  }

  return (
    <span
      className="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium"
      style={{ background: bg, color }}
    >
      {status}
    </span>
  )
}
