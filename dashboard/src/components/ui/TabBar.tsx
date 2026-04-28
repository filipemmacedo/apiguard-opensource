interface Tab {
  id: string
  label: string
  count?: number
  countVariant?: 'default' | 'warning'
}

interface TabBarProps {
  tabs: Tab[]
  activeTab: string
  onTabChange: (tabId: string) => void
}

export function TabBar({ tabs, activeTab, onTabChange }: TabBarProps) {
  return (
    <div
      role="tablist"
      className="flex gap-1 overflow-x-auto rounded-xl border border-[#E5E7EB] bg-white p-1"
    >
      {tabs.map((tab) => {
        const isActive = tab.id === activeTab
        const isWarning = tab.countVariant === 'warning' && (tab.count ?? 0) > 0
        return (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={isActive}
            onClick={() => onTabChange(tab.id)}
            className={`flex shrink-0 items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors ${
              isActive
                ? 'bg-[#00C896] text-white'
                : 'text-[#6B7280] hover:bg-[#F3F4F6] hover:text-[#374151]'
            }`}
          >
            {tab.label}
            {tab.count !== undefined && (
              <span
                className={`rounded-full px-1.5 py-0.5 text-[10px] font-semibold ${
                  isActive
                    ? 'bg-white/20 text-white'
                    : isWarning
                      ? 'bg-amber-100 text-amber-800'
                      : 'bg-[#F3F4F6] text-[#6B7280]'
                }`}
              >
                {tab.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
