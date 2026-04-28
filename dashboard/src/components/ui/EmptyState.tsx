import { InboxIcon } from 'lucide-react'

interface EmptyStateAction {
  label: string
  onClick: () => void
  primary?: boolean
}

interface EmptyStateProps {
  icon?: React.ReactNode
  title: string
  description: string
  actions?: EmptyStateAction[]
}

export function EmptyState({ icon, title, description, actions }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-[#E5E7EB] bg-white px-8 py-16 text-center">
      <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-[#F3F4F6]">
        {icon || <InboxIcon size={28} className="text-[#9CA3AF]" />}
      </div>
      <h3
        className="mb-2 text-lg font-semibold text-[#111827]"
        style={{ fontFamily: "'Space Grotesk', sans-serif" }}
      >
        {title}
      </h3>
      <p className="mb-6 max-w-sm text-sm text-[#6B7280]">{description}</p>
      {actions && actions.length > 0 && (
        <div className="flex items-center gap-3">
          {actions.map((action) => (
            <button
              key={action.label}
              type="button"
              onClick={action.onClick}
              className={`rounded-lg px-4 py-2 text-sm font-medium transition-colors ${
                action.primary
                  ? 'bg-[#00C896] text-white hover:bg-[#00B085]'
                  : 'border border-[#E5E7EB] bg-white text-[#374151] hover:bg-[#F9FAFB]'
              }`}
            >
              {action.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
