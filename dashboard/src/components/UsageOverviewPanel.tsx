import { useEffect, useMemo, useState } from 'react'
import { Activity, Coins, Users, Euro, BarChart3 } from 'lucide-react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  ResponsiveContainer,
  CartesianGrid,
  type TooltipContentProps,
  type TooltipPayloadEntry,
  type TooltipValueType,
} from 'recharts'
import {
  fetchAdminUserKeys,
  fetchDashboardLogsOverview,
  type AdminUserKey,
  type DashboardLogRecord,
  type DashboardUserLogsOverview,
} from '../lib/api'
import { buildDashboardTimeRange } from '../lib/timeRange'
import { StatsCard } from './ui/StatsCard'
import { DataTable, type Column } from './ui/DataTable'
import { EmptyState } from './ui/EmptyState'
import { FilterBar } from './ui/FilterBar'
import { isSecurityBlockedLog } from './LogsOverviewPanel'

export type FlatUsageLog = DashboardLogRecord & {
  user_id: string
  key_alias: string
  user_display_name: string
}

type GroupedUsageRow = {
  id: string
  user_id: string
  user_display_name: string
  key_alias: string
  estimated_cost_eur: number
  model: string
  latest_timestamp: string
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
}

const CHART_COLORS = [
  '#00C896', '#6366F1', '#F59E0B', '#EF4444', '#8B5CF6',
  '#EC4899', '#14B8A6', '#F97316', '#06B6D4', '#84CC16',
]

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return n.toLocaleString()
}

function formatCostEUR(eur: number): string {
  if (eur === 0) return '€0.0000'
  if (eur < 0.0001) return `€${eur.toFixed(6)}`
  return `€${eur.toFixed(4)}`
}

type ChartDataPoint = {
  date: string
  [key: string]: string | number
}

type UsageTooltipEntry = TooltipPayloadEntry<TooltipValueType>
type UsageTooltipProps = TooltipContentProps<TooltipValueType>

type UsageLegendEntry = {
  color?: string
  dataKey?: string | number
  value?: string | number
}

type AppliedChartRange = {
  fromValue: string
  toValue: string
}

function parseChartInputDate(value: string): Date | null {
  const trimmed = value.trim()
  if (!trimmed) return null

  if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) {
    const [year, month, day] = trimmed.split('-').map(Number)
    return new Date(year, month - 1, day)
  }

  const parsed = new Date(trimmed)
  if (Number.isNaN(parsed.valueOf())) {
    return null
  }

  return new Date(parsed.getFullYear(), parsed.getMonth(), parsed.getDate())
}

function startOfChartDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

function chartDayKey(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function formatChartDayLabel(date: Date, includeYear: boolean): string {
  return date.toLocaleDateString('en-US', includeYear
    ? { month: 'short', day: 'numeric', year: 'numeric' }
    : { month: 'short', day: 'numeric' })
}

function enumerateChartDays(start: Date, end: Date): Date[] {
  const days: Date[] = []
  const cursor = startOfChartDay(start)
  const lastDay = startOfChartDay(end)

  while (cursor.getTime() <= lastDay.getTime()) {
    days.push(new Date(cursor))
    cursor.setDate(cursor.getDate() + 1)
  }

  return days
}

function resolveUsageUserDisplayName(
  userKeys: AdminUserKey[],
  userID: string,
  maskedKey: string,
): string {
  const normalizedUserID = userID.trim()
  const normalizedMaskedKey = maskedKey.trim()
  const matchingKeys = userKeys.filter((userKey) => userKey.user_id === normalizedUserID)

  const exactMatch = matchingKeys.find((userKey) => userKey.masked_key === normalizedMaskedKey)
  if (exactMatch?.display_name.trim()) {
    return exactMatch.display_name.trim()
  }

  const activeNamedKey = matchingKeys.find(
    (userKey) => userKey.status === 'active' && userKey.display_name.trim(),
  )
  if (activeNamedKey?.display_name.trim()) {
    return activeNamedKey.display_name.trim()
  }

  const namedKey = matchingKeys.find((userKey) => userKey.display_name.trim())
  if (namedKey?.display_name.trim()) {
    return namedKey.display_name.trim()
  }

  return normalizedUserID
}

function buildChartData(logs: FlatUsageLog[], appliedRange?: AppliedChartRange): {
  data: ChartDataPoint[]
  topUsers: string[]
  truncated: boolean
  userLabels: Record<string, string>
} {
  const byDayUser: Record<string, Record<string, number>> = {}
  const userTotals: Record<string, number> = {}
  const userLabels: Record<string, string> = {}
  let minLogDay: Date | null = null
  let maxLogDay: Date | null = null

  for (const log of logs) {
    const tokens = log.total_tokens ?? 0
    const logDay = startOfChartDay(new Date(log.timestamp))
    const dayKey = chartDayKey(logDay)
    if (!byDayUser[dayKey]) byDayUser[dayKey] = {}
    byDayUser[dayKey][log.user_id] = (byDayUser[dayKey][log.user_id] ?? 0) + tokens
    userTotals[log.user_id] = (userTotals[log.user_id] ?? 0) + tokens
    userLabels[log.user_id] = log.user_display_name || log.user_id

    if (minLogDay == null || logDay.getTime() < minLogDay.getTime()) {
      minLogDay = logDay
    }
    if (maxLogDay == null || logDay.getTime() > maxLogDay.getTime()) {
      maxLogDay = logDay
    }
  }

  const allUsers = Object.entries(userTotals).sort((a, b) => b[1] - a[1])
  const topUsers = allUsers.slice(0, 10).map(([id]) => id)
  const truncated = allUsers.length > 10

  const appliedFromDay = parseChartInputDate(appliedRange?.fromValue ?? '')
  const appliedToDay = parseChartInputDate(appliedRange?.toValue ?? '')
  const rangeStart = appliedFromDay ?? minLogDay
  const rangeEnd = appliedToDay ?? maxLogDay

  const allDays = rangeStart && rangeEnd
    ? enumerateChartDays(rangeStart, rangeEnd)
    : Object.keys(byDayUser)
        .map((day) => parseChartInputDate(day))
        .filter((day): day is Date => day !== null)
        .sort((a, b) => a.getTime() - b.getTime())

  const includeYear = Boolean(
    rangeStart &&
      rangeEnd &&
      rangeStart.getFullYear() !== rangeEnd.getFullYear(),
  )

  const data: ChartDataPoint[] = allDays.map((day) => {
    const dayKey = chartDayKey(day)
    const point: ChartDataPoint = { date: formatChartDayLabel(day, includeYear) }
    for (const uid of topUsers) {
      point[uid] = byDayUser[dayKey]?.[uid] ?? 0
    }
    return point
  })

  return { data, topUsers, truncated, userLabels }
}

function renderUsageTooltip({
  active,
  label,
  payload,
  hoveredSeriesKey,
}: UsageTooltipProps & { hoveredSeriesKey: string | null }) {
  if (!active || !hoveredSeriesKey || !payload?.length) {
    return null
  }

  const hoveredEntry = payload.find((entry) => String(entry.dataKey) === hoveredSeriesKey)
  if (!hoveredEntry || hoveredEntry.value == null) {
    return null
  }

  const numericValue =
    typeof hoveredEntry.value === 'number'
      ? hoveredEntry.value
      : Number(hoveredEntry.value)

  if (!Number.isFinite(numericValue)) {
    return null
  }

  return (
    <div
      className="rounded-xl border border-[#E5E7EB] bg-white px-4 py-3 shadow-sm"
      style={{ fontSize: 12 }}
    >
      <div className="mb-1 text-sm font-medium text-[#111827]">{label}</div>
      <div style={{ color: hoveredEntry.color ?? '#111827' }}>
        {hoveredEntry.name}: {numericValue.toLocaleString()} tokens
      </div>
    </div>
  )
}

function renderUsageLegend({
  payload,
  selectedUserID,
  onUserSelect,
}: {
  payload?: UsageLegendEntry[]
  selectedUserID: string
  onUserSelect: (userID: string) => void
}) {
  if (!payload?.length) {
    return null
  }

  return (
    <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2 text-xs">
      {payload.map((entry) => {
        const userID = String(entry.dataKey ?? '')
        const isSelected = selectedUserID === userID

        return (
          <button
            key={userID}
            type="button"
            onClick={() => onUserSelect(userID)}
            className="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 transition-colors"
            style={{
              borderColor: isSelected ? entry.color ?? '#D1D5DB' : 'transparent',
              background: isSelected ? '#F9FAFB' : 'transparent',
              color: entry.color ?? '#374151',
            }}
          >
            <span
              className="h-2.5 w-2.5 rounded-full"
              style={{ background: entry.color ?? '#9CA3AF' }}
            />
            <span>{entry.value}</span>
          </button>
        )
      })}
    </div>
  )
}

function UsageTrendChart({
  logs,
  appliedRange,
  selectedUserID,
  onUserSelect,
}: {
  logs: FlatUsageLog[]
  appliedRange?: AppliedChartRange
  selectedUserID: string
  onUserSelect: (userID: string) => void
}) {
  const { data, topUsers, truncated, userLabels } = useMemo(
    () => buildChartData(logs, appliedRange),
    [appliedRange, logs],
  )
  const [hoveredSeriesKey, setHoveredSeriesKey] = useState<string | null>(null)

  if (data.length === 0) return null

  function suppressChartFocus(event: React.MouseEvent<HTMLDivElement>) {
    const target = event.target
    if (!(target instanceof Element) || !target.closest('.recharts-surface')) return

    event.preventDefault()

    const activeElement = document.activeElement as
      | (HTMLElement & { blur?: () => void })
      | (SVGElement & { blur?: () => void })
      | null

    activeElement?.blur?.()
  }

  return (
    <div className="rounded-xl border border-[#E5E7EB] bg-white p-6">
      <div className="mb-1 flex items-center justify-between">
        <div>
          <h3
            className="text-sm font-semibold text-[#111827]"
            style={{ fontFamily: "'Space Grotesk', sans-serif" }}
          >
            Token Usage Over Time
          </h3>
          <p className="text-xs text-[#9CA3AF]">Daily token consumption per user</p>
        </div>
        {truncated && (
          <span className="text-xs text-[#9CA3AF]">Showing top 10 users by token usage. Click a legend item to filter.</span>
        )}
      </div>
      <div
        className="usage-trend-chart mt-4 [&_svg]:outline-none [&_.recharts-wrapper]:outline-none"
        style={{ height: 240 }}
        onMouseDownCapture={suppressChartFocus}
        onMouseLeave={() => setHoveredSeriesKey(null)}
      >
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={data}
            margin={{ top: 4, right: 16, left: 0, bottom: 0 }}
            accessibilityLayer={false}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="#F3F4F6" />
            <XAxis
              dataKey="date"
              tick={{ fontSize: 11, fill: '#9CA3AF' }}
              tickLine={false}
              axisLine={false}
              interval="preserveStartEnd"
              minTickGap={24}
            />
            <YAxis
              tick={{ fontSize: 11, fill: '#9CA3AF' }}
              tickLine={false}
              axisLine={false}
              tickFormatter={formatTokens}
              width={48}
            />
             <Tooltip
               cursor={false}
               wrapperStyle={{ pointerEvents: 'none' }}
               content={(props) => renderUsageTooltip({
                 ...props,
                 hoveredSeriesKey,
               })}
             />
            <Legend
              wrapperStyle={{ fontSize: 12, paddingTop: 12 }}
              iconType="circle"
              iconSize={8}
              formatter={(value: string) => userLabels[value] ?? value}
              content={(props) => renderUsageLegend({
                payload: props.payload as UsageLegendEntry[] | undefined,
                selectedUserID,
                onUserSelect,
              })}
            />
            {topUsers.map((uid, i) => (
              <Line
                key={uid}
                type="monotone"
                dataKey={uid}
                name={userLabels[uid] ?? uid}
                stroke={CHART_COLORS[i % CHART_COLORS.length]}
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 5, stroke: '#FFFFFF', strokeWidth: 2 }}
                style={{ cursor: 'pointer' }}
                onClick={() => onUserSelect(uid)}
                onMouseEnter={() => setHoveredSeriesKey(uid)}
                onMouseMove={() => setHoveredSeriesKey(uid)}
                onMouseLeave={() => setHoveredSeriesKey((current) => (current === uid ? null : current))}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}

function groupUsageLogs(logs: FlatUsageLog[]): GroupedUsageRow[] {
  const grouped = new Map<string, GroupedUsageRow>()

  for (const log of logs) {
    const model = (log.model || 'n/a').trim() || 'n/a'
    const groupKey = `${log.user_id}::${model}`
    const existing = grouped.get(groupKey)

    if (existing) {
      existing.prompt_tokens += log.prompt_tokens ?? 0
      existing.completion_tokens += log.completion_tokens ?? 0
      existing.total_tokens += log.total_tokens ?? 0
      existing.estimated_cost_eur += log.estimated_cost_eur ?? 0

      if (new Date(log.timestamp).getTime() > new Date(existing.latest_timestamp).getTime()) {
        existing.latest_timestamp = log.timestamp
      }
      continue
    }

    grouped.set(groupKey, {
      id: groupKey,
      user_id: log.user_id,
      user_display_name: log.user_display_name,
      key_alias: log.key_alias,
      model,
      latest_timestamp: log.timestamp,
      prompt_tokens: log.prompt_tokens ?? 0,
      completion_tokens: log.completion_tokens ?? 0,
      total_tokens: log.total_tokens ?? 0,
      estimated_cost_eur: log.estimated_cost_eur ?? 0,
    })
  }

  return Array.from(grouped.values())
}

function applySortGroupedUsage(
  rows: GroupedUsageRow[],
  key: string,
  dir: 'asc' | 'desc',
): GroupedUsageRow[] {
  return [...rows].sort((a, b) => {
    let diff = 0
    if (key === 'user') diff = a.user_display_name.localeCompare(b.user_display_name)
    else if (key === 'model') diff = a.model.localeCompare(b.model)
    else if (key === 'tokens') diff = a.total_tokens - b.total_tokens
    else if (key === 'cost') diff = a.estimated_cost_eur - b.estimated_cost_eur
    else if (key === 'prompt') diff = a.prompt_tokens - b.prompt_tokens
    else if (key === 'completion') diff = a.completion_tokens - b.completion_tokens
    else if (key === 'time') diff = new Date(a.latest_timestamp).getTime() - new Date(b.latest_timestamp).getTime()
    return dir === 'desc' ? -diff : diff
  })
}

export default function UsageOverviewPanel({
  onDataLoaded,
}: { onDataLoaded?: (data: FlatUsageLog[]) => void } = {}) {
  const [userLogs, setUserLogs] = useState<DashboardUserLogsOverview[]>([])
  const [userKeys, setUserKeys] = useState<AdminUserKey[]>([])
  const [fromValue, setFromValue] = useState('')
  const [toValue, setToValue] = useState('')
  const [appliedFromValue, setAppliedFromValue] = useState('')
  const [appliedToValue, setAppliedToValue] = useState('')
  const [searchValue, setSearchValue] = useState('')
  const [userFilter, setUserFilter] = useState('')
  const [securityFilter, setSecurityFilter] = useState('')
  const [sortKey, setSortKey] = useState('tokens')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function loadUsage() {
    const nextFromValue = fromValue.trim()
    const nextToValue = toValue.trim()
    const timeRange = buildDashboardTimeRange(nextFromValue, nextToValue)
    if (timeRange.error) {
      setError(timeRange.error)
      return
    }

    setLoading(true)
    setError(null)
    try {
      const [logsPayload, userKeysPayload] = await Promise.all([
        fetchDashboardLogsOverview(timeRange.range),
        fetchAdminUserKeys().catch(() => ({ user_keys: [] as AdminUserKey[] })),
      ])
      setUserLogs(logsPayload.users)
      setUserKeys(userKeysPayload.user_keys)
      setAppliedFromValue(nextFromValue)
      setAppliedToValue(nextToValue)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load usage')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadUsage()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const allLogs = useMemo<FlatUsageLog[]>(() => {
    const flat: FlatUsageLog[] = []
    for (const user of userLogs) {
      const userDisplayName = resolveUsageUserDisplayName(userKeys, user.user_id, user.key_alias)
      for (const log of user.logs) {
        flat.push({
          ...log,
          user_id: user.user_id,
          key_alias: user.key_alias,
          user_display_name: userDisplayName,
        })
      }
    }
    flat.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
    return flat
  }, [userKeys, userLogs])

  useEffect(() => {
    onDataLoaded?.(allLogs)
  }, [allLogs, onDataLoaded])

  function handleSort(key: string) {
    if (key === sortKey) {
      setSortDir((d) => (d === 'desc' ? 'asc' : 'desc'))
    } else {
      setSortKey(key)
      setSortDir('desc')
    }
  }

  function handleChartUserSelect(userID: string) {
    setUserFilter((current) => (current === userID ? '' : userID))
  }

  const filteredLogs = useMemo(() => {
    let result = allLogs

    if (searchValue) {
      const q = searchValue.toLowerCase()
      result = result.filter(
        (l) =>
          l.user_display_name.toLowerCase().includes(q) ||
          l.user_id.toLowerCase().includes(q) ||
          l.key_alias.toLowerCase().includes(q) ||
          (l.model || '').toLowerCase().includes(q),
      )
    }

    if (userFilter) {
      result = result.filter((l) => l.user_id === userFilter)
    }

    if (securityFilter === 'pii') {
      result = result.filter(
        (l) =>
          (l.pii_summary?.ingress_finding_count ?? 0) +
            (l.pii_summary?.egress_finding_count ?? 0) >
          0,
      )
    } else if (securityFilter === 'blocked') {
      result = result.filter(isSecurityBlockedLog)
    } else if (securityFilter === 'clean') {
      result = result.filter(
        (l) =>
          !isSecurityBlockedLog(l) &&
          (l.pii_summary?.ingress_finding_count ?? 0) +
            (l.pii_summary?.egress_finding_count ?? 0) ===
            0,
      )
    }
    return result
  }, [allLogs, searchValue, userFilter, securityFilter])

  const groupedRows = useMemo(
    () => applySortGroupedUsage(groupUsageLogs(filteredLogs), sortKey, sortDir),
    [filteredLogs, sortKey, sortDir],
  )

  const groupedTotalCost = useMemo(
    () => formatCostEUR(groupedRows.reduce((sum, row) => sum + row.estimated_cost_eur, 0)),
    [groupedRows],
  )

  const userOptions = useMemo(() => {
    const usersByID = new Map<string, string>()
    for (const log of allLogs) {
      if (!usersByID.has(log.user_id)) {
        usersByID.set(log.user_id, log.user_display_name)
      }
    }

    return [
      { label: 'All Users', value: '' },
      ...Array.from(usersByID.entries())
        .sort((left, right) => left[1].localeCompare(right[1]))
        .map(([userID, displayName]) => ({
          label: displayName,
          value: userID,
        })),
    ]
  }, [allLogs])

  const stats = useMemo(() => {
    const totalTokens = filteredLogs.reduce((s, l) => s + (l.total_tokens ?? 0), 0)
    const uniqueUsers = new Set(filteredLogs.map((l) => l.user_id)).size
    return {
      totalRequests: filteredLogs.length.toLocaleString(),
      totalTokens: formatTokens(totalTokens),
      uniqueUsers: String(uniqueUsers),
      estCost: formatCostEUR(filteredLogs.reduce((s, l) => s + (l.estimated_cost_eur ?? 0), 0)),
    }
  }, [filteredLogs])

  const columns: Column<GroupedUsageRow>[] = [
    {
      key: 'user',
      header: 'User',
      sortable: true,
      render: (row) => (
        <div>
          <div className="text-sm font-medium text-[#111827]">{row.user_display_name}</div>
          <div className="text-xs text-[#9CA3AF]">{row.key_alias}</div>
        </div>
      ),
    },
    {
      key: 'model',
      header: 'Model',
      width: 160,
      sortable: true,
      render: (row) => (
        <span className="text-sm text-[#374151]">{row.model}</span>
      ),
    },
    {
      key: 'prompt',
      header: 'Prompt',
      width: 110,
      sortable: true,
      render: (row) => (
        <span className="text-sm text-[#6B7280]">
          {row.prompt_tokens?.toLocaleString() ?? '—'}
        </span>
      ),
    },
    {
      key: 'completion',
      header: 'Completion',
      width: 110,
      sortable: true,
      render: (row) => (
        <span className="text-sm text-[#6B7280]">
          {row.completion_tokens?.toLocaleString() ?? '—'}
        </span>
      ),
    },
    {
      key: 'tokens',
      header: 'Total Tokens',
      width: 120,
      sortable: true,
      render: (row) => (
        <span className="text-sm text-[#374151]">
          {row.total_tokens?.toLocaleString() ?? '—'}
        </span>
      ),
    },
    {
      key: 'cost',
      header: 'Est. Cost',
      width: 110,
      align: 'right',
      sortable: true,
      render: (row) => (
        <span className="text-sm font-medium text-[#111827]">
          {row.estimated_cost_eur != null ? formatCostEUR(row.estimated_cost_eur) : '—'}
        </span>
      ),
    },
  ]

  const hasData = userLogs.length > 0

  return (
    <div className="flex flex-col gap-5">
      {/* Filter Bar */}
      <FilterBar
        searchValue={searchValue}
        onSearchChange={setSearchValue}
        searchPlaceholder="Search by user, key or model..."
        dropdowns={[
          {
            label: 'User',
            value: userFilter,
            onChange: setUserFilter,
            options: userOptions,
          },
          {
            label: 'Security',
            value: securityFilter,
            onChange: setSecurityFilter,
            options: [
              { label: 'All Security', value: '' },
              { label: 'PII Detected', value: 'pii' },
              { label: 'Security Block', value: 'blocked' },
              { label: 'Clean', value: 'clean' },
            ],
          },
        ]}
        fromValue={fromValue}
        onFromChange={setFromValue}
        toValue={toValue}
        onToChange={setToValue}
        onApply={loadUsage}
        loading={loading}
      />

      {/* Stats Row */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatsCard
          label="Total Requests"
          value={stats.totalRequests}
          icon={Activity}
        />
        <StatsCard
          label="Total Tokens"
          value={stats.totalTokens}
          icon={Coins}
          iconColor="#F59E0B"
          iconBg="#FFFBEB"
        />
        <StatsCard
          label="Unique Users"
          value={stats.uniqueUsers}
          icon={Users}
          iconColor="#8B5CF6"
          iconBg="#F5F3FF"
        />
        <StatsCard
          label="Est. Cost"
          value={stats.estCost}
          icon={Euro}
          iconColor="#059669"
          iconBg="#ECFDF5"
        />
      </div>

      {error && <p className="text-sm text-red-600">{error}</p>}

      {/* Usage Trend Chart */}
      {hasData && (
        <div className="hidden sm:block">
          <UsageTrendChart
            logs={filteredLogs}
            appliedRange={{ fromValue: appliedFromValue, toValue: appliedToValue }}
            selectedUserID={userFilter}
            onUserSelect={handleChartUserSelect}
          />
        </div>
      )}

      {/* Table or Empty State */}
      {loading && !hasData ? (
        <div className="flex items-center justify-center rounded-xl border border-[#E5E7EB] bg-white py-20">
          <span className="text-sm text-[#9CA3AF]">Loading usage...</span>
        </div>
      ) : hasData ? (
        <DataTable
          columns={columns}
          data={groupedRows}
          pageSize={15}
          title="Request Cost Log"
          subtitle="Grouped token consumption by user and model"
          badge={`${groupedRows.length} user/model groups`}
          footer={(
            <div className="flex justify-end">
              <span className="text-sm font-semibold text-[#111827]">
                Total Cost: {groupedTotalCost}
              </span>
            </div>
          )}
          keyExtractor={(row) => row.id}
          sortKey={sortKey}
          sortDir={sortDir}
          onSort={handleSort}
        />
      ) : (
        <EmptyState
          icon={<BarChart3 size={28} className="text-[#9CA3AF]" />}
          title="No usage data available"
          description="Usage data will appear here once your users start making API requests."
          actions={[
            { label: 'Manage API Keys', onClick: () => window.location.assign('/api-management'), primary: true },
          ]}
        />
      )}
    </div>
  )
}
