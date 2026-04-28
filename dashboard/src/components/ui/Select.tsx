import { useState, useRef, useEffect } from 'react'
import { ChevronDown, Check } from 'lucide-react'

export interface SelectOption {
  value: string
  label: string
  description?: string
}

interface SelectProps {
  value: string
  onChange: (value: string) => void
  options: SelectOption[]
  placeholder?: string
  disabled?: boolean
  className?: string
  searchable?: boolean
  searchPlaceholder?: string
  emptyMessage?: string
  'aria-label'?: string
}

export function Select({
  value,
  onChange,
  options,
  placeholder = 'Select...',
  disabled = false,
  className = '',
  searchable = false,
  searchPlaceholder = 'Search...',
  emptyMessage = 'No options found.',
  'aria-label': ariaLabel,
}: SelectProps) {
  const [open, setOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const ref = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  const selected = options.find((o) => o.value === value)
  const normalizedQuery = searchQuery.trim().toLowerCase()
  const filteredOptions =
    searchable && normalizedQuery !== ''
      ? options.filter((option) =>
          [option.label, option.description ?? '', option.value]
            .join(' ')
            .toLowerCase()
            .includes(normalizedQuery),
        )
      : options

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  useEffect(() => {
    if (!open) {
      setSearchQuery('')
      return
    }
    if (searchable) {
      searchRef.current?.focus()
    }
  }, [open, searchable])

  function handleSelect(opt: SelectOption) {
    onChange(opt.value)
    setOpen(false)
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Escape') {
      setOpen(false)
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      setOpen(!open)
    } else if (e.key === 'ArrowDown' && !open) {
      e.preventDefault()
      setOpen(true)
    }
  }

  return (
    <div ref={ref} className={`relative ${className}`}>
      <button
        type="button"
        role="combobox"
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-label={ariaLabel}
        disabled={disabled}
        onClick={() => !disabled && setOpen(!open)}
        onKeyDown={handleKeyDown}
        className={`flex h-10 w-full items-center justify-between rounded-lg border bg-[#F9FAFB] px-3 text-sm transition ${
          disabled
            ? 'cursor-not-allowed opacity-60'
            : open
              ? 'border-[#00C896] ring-1 ring-[#00C896]/30'
              : 'border-[#D1D5DB] hover:border-[#9CA3AF]'
        }`}
      >
        <span className={selected ? 'text-[#374151]' : 'text-[#9CA3AF]'}>
          {selected?.label || placeholder}
        </span>
        <ChevronDown
          className={`h-3.5 w-3.5 text-[#9CA3AF] transition-transform ${open ? 'rotate-180' : ''}`}
        />
      </button>

      {open && (
        <div className="absolute z-50 mt-1 w-full overflow-hidden rounded-lg border border-[#E5E7EB] bg-white shadow-lg shadow-black/8" onClick={(e) => e.stopPropagation()}>
          {searchable && (
            <div className="border-b border-[#E5E7EB] p-2">
              <input
                ref={searchRef}
                type="text"
                value={searchQuery}
                onChange={(event) => setSearchQuery(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Escape') {
                    setOpen(false)
                  }
                  if (event.key === 'Enter' && filteredOptions.length === 1) {
                    event.preventDefault()
                    handleSelect(filteredOptions[0])
                  }
                }}
                placeholder={searchPlaceholder}
                className="h-9 w-full rounded-md border border-[#D1D5DB] bg-[#F9FAFB] px-3 text-sm text-[#374151] outline-none focus:border-[#00C896] focus:ring-1 focus:ring-[#00C896]"
              />
            </div>
          )}
          {filteredOptions.length === 0 ? (
            <div className="px-3 py-3 text-sm text-[#9CA3AF]">{emptyMessage}</div>
          ) : (
            <ul role="listbox" className="max-h-64 overflow-auto">
              {filteredOptions.map((opt) => {
                const isSelected = opt.value === value
                return (
                  <li
                    key={opt.value}
                    role="option"
                    aria-selected={isSelected}
                    onMouseDown={(event) => {
                      // Prevent parent labels from re-activating the combobox trigger.
                      event.preventDefault()
                    }}
                    onClick={(event) => {
                      event.preventDefault()
                      event.stopPropagation()
                      handleSelect(opt)
                    }}
                    className={`flex cursor-pointer items-center gap-2.5 px-3 py-2.5 text-sm transition-colors ${
                      isSelected
                        ? 'bg-[#00C896]/8 text-[#00C896]'
                        : 'text-[#374151] hover:bg-[#F9FAFB]'
                    }`}
                  >
                    <div className="min-w-0 flex-1">
                      <div className={`font-medium ${isSelected ? 'text-[#00C896]' : ''}`}>
                        {opt.label}
                      </div>
                      {opt.description && (
                        <div className="mt-0.5 text-xs text-[#9CA3AF]">{opt.description}</div>
                      )}
                    </div>
                    {isSelected && <Check className="h-4 w-4 shrink-0 text-[#00C896]" />}
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}
