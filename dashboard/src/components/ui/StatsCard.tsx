import type { LucideIcon } from 'lucide-react'

interface StatsCardProps {
  label: string
  value: string
  icon: LucideIcon
  iconColor?: string
  iconBg?: string
  delta?: string
  deltaType?: 'positive' | 'negative' | 'neutral'
}

export function StatsCard({
  label,
  value,
  icon: Icon,
  iconColor = '#00C896',
  iconBg = '#ECFDF5',
  delta,
  deltaType = 'neutral',
}: StatsCardProps) {
  const deltaColor =
    deltaType === 'positive'
      ? '#22C55E'
      : deltaType === 'negative'
        ? '#EF4444'
        : '#6B7280'

  return (
    <div className="flex flex-col gap-2.5 rounded-xl bg-white p-[18px] border border-[#E5E7EB]">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-[#6B7280]">{label}</span>
        <div
          className="flex h-[30px] w-[30px] items-center justify-center rounded-lg"
          style={{ background: iconBg }}
        >
          <Icon size={14} style={{ color: iconColor }} />
        </div>
      </div>
      <span
        className="text-2xl font-bold text-[#111827]"
        style={{ fontFamily: "'Space Grotesk', sans-serif" }}
      >
        {value}
      </span>
      {delta && (
        <div className="flex items-center gap-1">
          <span
            className="h-1.5 w-1.5 rounded-full"
            style={{ background: deltaColor }}
          />
          <span className="text-[11px]" style={{ color: deltaColor }}>
            {delta}
          </span>
        </div>
      )}
    </div>
  )
}
