import { useEffect, useMemo, useState } from 'react'
import { Activity, CheckCircle, XCircle, Clock, FileText, ShieldAlert } from 'lucide-react'
import {
  fetchDashboardLogsOverview,
  type DashboardLogRecord,
  type DashboardUserLogsOverview,
} from '../lib/api'
import { buildDashboardTimeRange } from '../lib/timeRange'
import { StatsCard } from './ui/StatsCard'
import { StatusBadge } from './ui/StatusBadge'
import { DataTable, type Column } from './ui/DataTable'
import { EmptyState } from './ui/EmptyState'
import { FilterBar } from './ui/FilterBar'

export type FlatLog = DashboardLogRecord & { user_id: string }
type SecurityAwareLog = Pick<
  DashboardLogRecord,
  'status' | 'latency_ms' | 'security_summary' | 'pii_summary'
>

export function isSecurityBlockedLog(log: Pick<DashboardLogRecord, 'status' | 'security_summary' | 'pii_summary'>) {
  const hasPII =
    (log.pii_summary?.ingress_finding_count ?? 0) +
      (log.pii_summary?.egress_finding_count ?? 0) >
    0
  const hasSecurityLabels = (log.security_summary?.labels?.length ?? 0) > 0
  return log.status >= 400 && log.status < 500 && (hasPII || hasSecurityLabels)
}

export function buildLogsOverviewStats(logs: SecurityAwareLog[]) {
  const totalCount = logs.length
  const successCount = logs.filter((log) => log.status >= 200 && log.status < 300).length
  const securityBlockCount = logs.filter(isSecurityBlockedLog).length
  const errorCount = logs.filter(
    (log) => log.status >= 400 && !isSecurityBlockedLog(log),
  ).length
  const avgLatency =
    totalCount > 0
      ? Math.round(logs.reduce((sum, log) => sum + log.latency_ms, 0) / totalCount)
      : 0

  return {
    total: totalCount.toLocaleString(),
    totalCount,
    successRate: totalCount > 0 ? `${((successCount / totalCount) * 100).toFixed(1)}%` : '0%',
    errorRate: totalCount > 0 ? `${((errorCount / totalCount) * 100).toFixed(1)}%` : '0%',
    securityBlockRate:
      totalCount > 0 ? `${((securityBlockCount / totalCount) * 100).toFixed(1)}%` : '0%',
    securityBlockCount,
    avgLatency: `${avgLatency}ms`,
  }
}

export default function LogsOverviewPanel({ onDataLoaded }: { onDataLoaded?: (logs: FlatLog[]) => void } = {}) {
  const [userLogs, setUserLogs] = useState<DashboardUserLogsOverview[]>([])
  const [fromValue, setFromValue] = useState('')
  const [toValue, setToValue] = useState('')
  const [searchValue, setSearchValue] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [userFilter, setUserFilter] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function loadOverviewLogs() {
    const timeRange = buildDashboardTimeRange(fromValue, toValue)
    if (timeRange.error) {
      setError(timeRange.error)
      return
    }

    setLoading(true)
    setError(null)
    try {
      const payload = await fetchDashboardLogsOverview(timeRange.range)
      setUserLogs(payload.users)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load logs overview')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadOverviewLogs()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const allLogs = useMemo<FlatLog[]>(() => {
    const flat: FlatLog[] = []
    for (const user of userLogs) {
      for (const log of user.logs) {
        flat.push({ ...log, user_id: user.user_id })
      }
    }
    flat.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
    return flat
  }, [userLogs])

  useEffect(() => {
    onDataLoaded?.(allLogs)
  }, [allLogs, onDataLoaded])

  const filteredLogs = useMemo(() => {
    let result = allLogs
    if (searchValue) {
      const q = searchValue.toLowerCase()
      result = result.filter(
        (l) => {
          const securityText = [
            ...(l.security_summary?.labels ?? []),
            ...(l.pii_summary?.entity_types ?? []),
            ...(l.pii_summary?.actions ?? []),
            isSecurityBlockedLog(l) ? 'blocked' : '',
          ]
            .join(' ')
            .toLowerCase()

          return (
          l.user_id.toLowerCase().includes(q) ||
          (l.model || '').toLowerCase().includes(q) ||
            String(l.status).includes(q) ||
            securityText.includes(q)
          )
        },
      )
    }
    if (userFilter) {
      result = result.filter((l) => l.user_id === userFilter)
    }
    if (statusFilter) {
      if (statusFilter === 'success') {
        result = result.filter((l) => l.status >= 200 && l.status < 300)
      } else if (statusFilter === 'error') {
        result = result.filter((l) => l.status >= 400 && !isSecurityBlockedLog(l))
      } else if (statusFilter === 'blocked') {
        result = result.filter(isSecurityBlockedLog)
      }
    }
    return result
  }, [allLogs, searchValue, userFilter, statusFilter])

  const stats = useMemo(() => buildLogsOverviewStats(allLogs), [allLogs])

  const userOptions = useMemo(() => {
    const ids = [...new Set(userLogs.map((user) => user.user_id))]
    return [
      { label: 'All Users', value: '' },
      ...ids.map((id) => ({ label: id, value: id })),
    ]
  }, [userLogs])

  const columns: Column<FlatLog>[] = [
    {
      key: 'time',
      header: 'Time',
      width: 200,
      render: (row) => (
        <span className="text-xs text-[#6B7280]">{new Date(row.timestamp).toLocaleString()}</span>
      ),
    },
    {
      key: 'user',
      header: 'User',
      width: 140,
      render: (row) => <span className="text-sm font-medium text-[#111827]">{row.user_id}</span>,
    },
    {
      key: 'model',
      header: 'Model',
      width: 180,
      render: (row) => <span className="text-sm text-[#374151]">{row.model || 'n/a'}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      width: 100,
      render: (row) => <LogStatusBadge log={row} />,
    },
    {
      key: 'security',
      header: 'Security',
      width: 180,
      render: (row) => (
        <div className="text-xs text-[#6B7280]">
          <LogSecuritySummary summary={row.security_summary} piiSummary={row.pii_summary} />
        </div>
      ),
    },
    {
      key: 'latency',
      header: 'Latency',
      width: 100,
      render: (row) => <span className="text-sm text-[#374151]">{row.latency_ms}ms</span>,
    },
    {
      key: 'tokens',
      header: 'Tokens',
      width: 100,
      align: 'right',
      render: (row) => (
        <span className="text-sm text-[#374151]">
          {row.total_tokens?.toLocaleString() ?? 'n/a'}
        </span>
      ),
    },
  ]

  const hasData = userLogs.length > 0

  return (
    <div className="flex flex-col gap-5">
      {/* Stats Row */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
        <StatsCard
          label="Total Requests"
          value={stats.total}
          icon={Activity}
          delta="+18% today"
          deltaType="positive"
        />
        <StatsCard
          label="Success Rate"
          value={stats.successRate}
          icon={CheckCircle}
          iconColor="#059669"
          iconBg="#ECFDF5"
          delta="Healthy"
          deltaType="positive"
        />
        <StatsCard
          label="Security Blocks"
          value={stats.securityBlockRate}
          icon={ShieldAlert}
          iconColor="#B45309"
          iconBg="#FFFBEB"
          delta={
            stats.securityBlockCount > 0
              ? `${stats.securityBlockCount} blocked`
              : 'No policy blocks'
          }
          deltaType={stats.securityBlockCount > 0 ? 'negative' : 'positive'}
        />
        <StatsCard
          label="Error Rate"
          value={stats.errorRate}
          icon={XCircle}
          iconColor="#DC2626"
          iconBg="#FEF2F2"
          delta={parseFloat(stats.errorRate) > 5 ? 'Above threshold' : 'Within limits'}
          deltaType={parseFloat(stats.errorRate) > 5 ? 'negative' : 'positive'}
        />
        <StatsCard
          label="Avg Latency"
          value={stats.avgLatency}
          icon={Clock}
          iconColor="#7C3AED"
          iconBg="#F5F3FF"
        />
      </div>

      {/* Filter Bar */}
      <FilterBar
        searchValue={searchValue}
        onSearchChange={setSearchValue}
        searchPlaceholder="Search by user, model, status, security..."
        dropdowns={[
          {
            label: 'User',
            value: userFilter,
            onChange: setUserFilter,
            options: userOptions,
          },
          {
            label: 'Status',
            value: statusFilter,
            onChange: setStatusFilter,
            options: [
              { label: 'All Status', value: '' },
              { label: 'Success (2xx)', value: 'success' },
              { label: 'System Error', value: 'error' },
              { label: 'Security Block', value: 'blocked' },
            ],
          },
        ]}
        fromValue={fromValue}
        onFromChange={setFromValue}
        toValue={toValue}
        onToChange={setToValue}
        onApply={loadOverviewLogs}
        loading={loading}
      />

      {error && <p className="text-sm text-red-600">{error}</p>}

      {/* Table or Empty State */}
      {loading && !hasData ? (
        <div className="flex items-center justify-center rounded-xl border border-[#E5E7EB] bg-white py-20">
          <span className="text-sm text-[#9CA3AF]">Loading logs...</span>
        </div>
      ) : hasData ? (
        <DataTable
          columns={columns}
          data={filteredLogs}
          pageSize={15}
          title="Request Logs"
          subtitle="Successful, blocked, and failed request activity across users"
          badge={`${filteredLogs.length} logs`}
          keyExtractor={(row) => row.request_id}
        />
      ) : (
        <EmptyState
          icon={<FileText size={28} className="text-[#9CA3AF]" />}
          title="No logs yet"
          description="Once your users start making API requests through the proxy, logs will appear here."
          actions={[
            { label: 'Set up API Keys', onClick: () => window.location.assign('/api-management'), primary: true },
            { label: 'View Docs', onClick: () => {} },
          ]}
        />
      )}
    </div>
  )
}

export function LogStatusBadge({
  log,
}: {
  log: Pick<DashboardLogRecord, 'status' | 'security_summary' | 'pii_summary'>
}) {
  if (isSecurityBlockedLog(log)) {
    return (
      <span
        className="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium"
        style={{ background: '#FFF7ED', color: '#D97706' }}
      >
        Blocked
      </span>
    )
  }

  return <StatusBadge status={log.status} />
}

export function LogSecuritySummary({
  summary,
  piiSummary,
}: {
  summary?: DashboardLogRecord['security_summary']
  piiSummary?: DashboardLogRecord['pii_summary']
}) {
  const labels = summary?.labels ?? []
  const hasPII =
    (piiSummary?.ingress_finding_count ?? 0) + (piiSummary?.egress_finding_count ?? 0) > 0 ||
    labels.includes('PII')
  const otherLabels = labels.filter((l) => l !== 'PII')

  if (!hasPII && otherLabels.length === 0) {
    return '-'
  }

  return (
    <>
      {hasPII ? (
        <>
          <div className="font-semibold text-[#111827]">PII</div>
          {piiSummary ? (
            <div>
              in {piiSummary.ingress_finding_count} / out {piiSummary.egress_finding_count}
            </div>
          ) : null}
        </>
      ) : null}
      {otherLabels.map((label) => (
        <div key={label} className="font-semibold text-[#111827]">{label}</div>
      ))}
    </>
  )
}
