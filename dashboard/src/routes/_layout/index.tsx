import { createFileRoute } from '@tanstack/react-router'
import { useCallback, useState } from 'react'
import { Download } from 'lucide-react'
import LogsOverviewPanel, { type FlatLog } from '#/components/LogsOverviewPanel'
import { TopBar } from '#/components/ui/TopBar'
import { downloadCSV } from '#/lib/csv'

export const Route = createFileRoute('/_layout/')({ component: LogsPage })

function LogsPage() {
  const [logs, setLogs] = useState<FlatLog[]>([])

  const handleDataLoaded = useCallback((data: FlatLog[]) => {
    setLogs(data)
  }, [])

  function exportLogs() {
    const date = new Date().toISOString().slice(0, 10)
    downloadCSV(`logs-${date}.csv`, [
      { header: 'Timestamp', accessor: (r) => r.timestamp },
      { header: 'User', accessor: (r) => r.user_id },
      { header: 'Model', accessor: (r) => r.model },
      { header: 'Status', accessor: (r) => r.status },
      { header: 'Latency (ms)', accessor: (r) => r.latency_ms },
      { header: 'Prompt Tokens', accessor: (r) => r.prompt_tokens ?? '' },
      { header: 'Completion Tokens', accessor: (r) => r.completion_tokens ?? '' },
      { header: 'Total Tokens', accessor: (r) => r.total_tokens ?? '' },
      { header: 'PII Ingress Findings', accessor: (r) => r.pii_summary?.ingress_finding_count ?? 0 },
      { header: 'PII Egress Findings', accessor: (r) => r.pii_summary?.egress_finding_count ?? 0 },
      { header: 'Security Labels', accessor: (r) => (r.security_summary?.labels ?? []).join('; ') },
    ], logs)
  }

  return (
    <div className="flex h-full flex-col" style={{ background: '#F8F9FA' }}>
      <TopBar title="Logs">
        <button
          type="button"
          disabled={logs.length === 0}
          onClick={exportLogs}
          className="flex items-center gap-1.5 rounded-lg border border-[#E5E7EB] bg-white px-3 py-1.5 text-sm font-medium text-[#374151] hover:bg-[#F9FAFB] disabled:opacity-50"
        >
          <Download size={14} />
          Export CSV
        </button>
      </TopBar>
      <div className="flex-1 overflow-auto p-8">
        <LogsOverviewPanel onDataLoaded={handleDataLoaded} />
      </div>
    </div>
  )
}
