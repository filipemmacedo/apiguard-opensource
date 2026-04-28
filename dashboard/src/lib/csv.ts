function escapeCSVValue(value: unknown): string {
  const str = value == null ? '' : String(value)
  if (str.includes(',') || str.includes('"') || str.includes('\n')) {
    return `"${str.replace(/"/g, '""')}"`
  }
  return str
}

export function downloadCSV<T>(
  filename: string,
  columns: { header: string; accessor: (row: T) => unknown }[],
  rows: T[],
) {
  const header = columns.map((c) => escapeCSVValue(c.header)).join(',')
  const lines = rows.map((row) =>
    columns.map((c) => escapeCSVValue(c.accessor(row))).join(','),
  )
  const csv = [header, ...lines].join('\n')
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}
