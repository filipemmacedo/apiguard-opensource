interface TopBarProps {
  title: string
  children?: React.ReactNode
}

export function TopBar({ title, children }: TopBarProps) {
  return (
    <div className="flex min-h-[60px] flex-wrap items-center justify-between gap-3 border-b border-[#E5E7EB] bg-white px-4 py-3 sm:px-8">
      <h1
        className="text-lg font-semibold text-[#111827]"
        style={{ fontFamily: "'Space Grotesk', sans-serif" }}
      >
        {title}
      </h1>
      {children && <div className="flex items-center gap-3">{children}</div>}
    </div>
  )
}
