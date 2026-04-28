import { Search } from 'lucide-react'
import { Select } from './Select'

export interface FilterDropdown {
  label: string
  value: string
  onChange: (value: string) => void
  options: { label: string; value: string }[]
}

interface FilterBarProps {
  searchValue?: string
  onSearchChange?: (value: string) => void
  searchPlaceholder?: string
  dropdowns?: FilterDropdown[]
  fromValue?: string
  onFromChange?: (value: string) => void
  toValue?: string
  onToChange?: (value: string) => void
  onApply: () => void
  loading?: boolean
}

export function FilterBar({
  searchValue,
  onSearchChange,
  searchPlaceholder = 'Search...',
  dropdowns,
  fromValue,
  onFromChange,
  toValue,
  onToChange,
  onApply,
  loading,
}: FilterBarProps) {
  return (
    <div className="flex flex-col gap-2.5 rounded-xl border border-[#E5E7EB] bg-white p-4 lg:flex-row lg:flex-wrap lg:items-center">
      {onSearchChange !== undefined && (
        <div className="flex min-w-0 flex-1 items-center gap-2 rounded-lg border border-[#D1D5DB] bg-[#F9FAFB] px-3 h-9 lg:min-w-[180px]">
          <Search size={14} className="text-[#9CA3AF] shrink-0" />
          <input
            type="text"
            value={searchValue ?? ''}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder={searchPlaceholder}
            className="w-full border-0 bg-transparent text-[13px] text-[#374151] placeholder-[#C4C4C4] outline-none"
          />
        </div>
      )}

      {dropdowns?.map((dd) => (
        <Select
          key={dd.label}
          aria-label={dd.label}
          value={dd.value}
          onChange={dd.onChange}
          options={dd.options}
          className="w-full lg:w-auto lg:min-w-[140px]"
        />
      ))}

      {onFromChange && (
        <input
          type="date"
          aria-label="From date"
          value={fromValue ?? ''}
          onChange={(e) => onFromChange(e.target.value)}
          className="h-9 w-full rounded-lg border border-[#D1D5DB] bg-[#F9FAFB] px-3 text-[13px] text-[#6B7280] outline-none lg:w-auto lg:min-w-[160px]"
        />
      )}
      {onToChange && (
        <input
          type="date"
          aria-label="To date"
          value={toValue ?? ''}
          onChange={(e) => onToChange(e.target.value)}
          className="h-9 w-full rounded-lg border border-[#D1D5DB] bg-[#F9FAFB] px-3 text-[13px] text-[#6B7280] outline-none lg:w-auto lg:min-w-[160px]"
        />
      )}

      <button
        type="button"
        onClick={onApply}
        disabled={loading}
        className="flex h-9 w-full items-center justify-center gap-1.5 rounded-lg bg-[#00C896] px-4 text-[13px] font-medium text-white hover:bg-[#00B085] disabled:opacity-60 lg:w-auto"
      >
        <Search size={13} />
        {loading ? 'Loading...' : 'Apply Filter'}
      </button>
    </div>
  )
}
