import { useState } from 'react'
import { ChevronLeft, ChevronRight, ChevronUp, ChevronDown } from 'lucide-react'

export interface Column<T> {
  key: string
  header: string
  width?: number | string
  align?: 'left' | 'right' | 'center'
  sortable?: boolean
  render: (row: T, index: number) => React.ReactNode
}

interface DataTableProps<T> {
  columns: Column<T>[]
  data: T[]
  pageSize?: number
  title?: string
  subtitle?: string
  badge?: string
  footer?: React.ReactNode
  keyExtractor: (row: T, index: number) => string
  sortKey?: string
  sortDir?: 'asc' | 'desc'
  onSort?: (key: string) => void
}

export function DataTable<T>({
  columns,
  data,
  pageSize = 10,
  title,
  subtitle,
  badge,
  footer,
  keyExtractor,
  sortKey,
  sortDir,
  onSort,
}: DataTableProps<T>) {
  const [page, setPage] = useState(0)
  const totalPages = Math.max(1, Math.ceil(data.length / pageSize))
  const start = page * pageSize
  const pageData = data.slice(start, start + pageSize)

  return (
    <div className="overflow-hidden rounded-xl border border-[#E5E7EB] bg-white">
      {(title || badge) && (
        <div className="flex items-center gap-2 border-b border-[#E5E7EB] px-4 py-4 sm:px-6">
          <div className="flex-1">
            {title && (
              <h3
                className="text-sm font-semibold text-[#111827]"
                style={{ fontFamily: "'Space Grotesk', sans-serif" }}
              >
                {title}
              </h3>
            )}
            {subtitle && (
              <p className="text-xs text-[#9CA3AF]">{subtitle}</p>
            )}
          </div>
          {badge && (
            <span className="rounded-full bg-[#F3F4F6] px-2.5 py-1 text-xs font-medium text-[#6B7280]">
              {badge}
            </span>
          )}
        </div>
      )}

      {/* Desktop table view (md+) */}
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full">
          <thead>
            <tr className="bg-[#F9FAFB]">
              {columns.map((col) => (
                <th
                  key={col.key}
                  scope="col"
                  onClick={col.sortable && onSort ? () => onSort(col.key) : undefined}
                  className={`border-b border-[#E5E7EB] px-6 py-2.5 text-[11px] font-semibold uppercase tracking-wider text-[#6B7280] ${col.sortable && onSort ? 'cursor-pointer select-none hover:text-[#374151]' : ''}`}
                  style={{
                    width: col.width,
                    textAlign: col.align || 'left',
                  }}
                >
                  {col.sortable && onSort ? (
                    <span className="inline-flex items-center gap-1">
                      {col.header}
                      {sortKey === col.key ? (
                        sortDir === 'asc' ? <ChevronUp size={12} /> : <ChevronDown size={12} />
                      ) : (
                        <span className="opacity-30"><ChevronDown size={12} /></span>
                      )}
                    </span>
                  ) : col.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {pageData.map((row, i) => (
              <tr
                key={keyExtractor(row, start + i)}
                className={`border-b border-[#E5E7EB] ${i % 2 === 1 ? 'bg-[#FAFAFA]' : 'bg-white'}`}
              >
                {columns.map((col) => (
                  <td
                    key={col.key}
                    className="px-6 py-3.5 text-sm text-[#111827]"
                    style={{
                      width: col.width,
                      textAlign: col.align || 'left',
                    }}
                  >
                    {col.render(row, start + i)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Mobile card view (<md) */}
      <div className="space-y-3 p-3 md:hidden">
        {pageData.map((row, i) => (
          <div
            key={keyExtractor(row, start + i)}
            className="rounded-lg border border-[#E5E7EB] bg-[#FAFAFA] p-3"
          >
            <dl className="space-y-1.5">
              {columns.map((col) => (
                <div key={col.key} className="flex items-start justify-between gap-2">
                  <dt className="text-[11px] font-semibold uppercase tracking-wider text-[#6B7280] shrink-0">
                    {col.header}
                  </dt>
                  <dd className="text-sm text-[#111827] text-right">
                    {col.render(row, start + i)}
                  </dd>
                </div>
              ))}
            </dl>
          </div>
        ))}
      </div>

      {footer && (
        <div className="border-t border-[#E5E7EB] bg-white px-4 py-3 sm:px-6">
          {footer}
        </div>
      )}

      <div className="flex items-center justify-between border-t border-[#E5E7EB] bg-[#F9FAFB] px-4 py-3 sm:px-6">
        <span className="text-xs text-[#6B7280]">
          Showing {data.length === 0 ? 0 : start + 1}–{Math.min(start + pageSize, data.length)} of{' '}
          {data.length.toLocaleString()}
        </span>
        <div className="flex items-center gap-4">
          <button
            type="button"
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={page === 0}
            className="flex items-center gap-1 text-xs font-medium text-[#6B7280] disabled:opacity-40"
          >
            <ChevronLeft size={14} /> Previous
          </button>
          <span className="text-xs text-[#6B7280]">
            {page + 1} / {totalPages}
          </span>
          <button
            type="button"
            onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
            disabled={page >= totalPages - 1}
            className="flex items-center gap-1 text-xs font-medium text-[#6B7280] disabled:opacity-40"
          >
            Next <ChevronRight size={14} />
          </button>
        </div>
      </div>
    </div>
  )
}
