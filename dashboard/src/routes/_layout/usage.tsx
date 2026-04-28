import { createFileRoute } from '@tanstack/react-router'
import { useCallback, useState } from 'react'
import { Download } from 'lucide-react'
import UsageOverviewPanel, { type FlatUsageLog } from '#/components/UsageOverviewPanel'
import { TopBar } from '#/components/ui/TopBar'
import { downloadCSV } from '#/lib/csv'

export const Route = createFileRoute('/_layout/usage')({
  component: UsagePage,
})

function UsagePage() {
  const [usageData, setUsageData] = useState<FlatUsageLog[]>([])

  const handleDataLoaded = useCallback((data: FlatUsageLog[]) => {
    setUsageData(data)
  }, [])

  function exportUsage() {
    const date = new Date().toISOString().slice(0, 10)
    downloadCSV(`usage-${date}.csv`, [
      { header: 'Time', accessor: (r) => new Date(r.timestamp).toLocaleString() },
      { header: 'User', accessor: (r) => r.user_display_name },
      { header: 'User ID', accessor: (r) => r.user_id },
      { header: 'API Key', accessor: (r) => r.key_alias },
      { header: 'Model', accessor: (r) => r.model || '' },
      { header: 'Prompt Tokens', accessor: (r) => r.prompt_tokens ?? 0 },
      { header: 'Completion Tokens', accessor: (r) => r.completion_tokens ?? 0 },
      { header: 'Total Tokens', accessor: (r) => r.total_tokens ?? 0 },
      { header: 'Est. Cost', accessor: (r) => {
        if (r.estimated_cost_eur == null) return '—'
        if (r.estimated_cost_eur === 0) return '€0.0000'
        if (r.estimated_cost_eur < 0.0001) return `€${r.estimated_cost_eur.toFixed(6)}`
        return `€${r.estimated_cost_eur.toFixed(4)}`
      }},
      { header: 'PII Ingress', accessor: (r) => r.pii_summary?.ingress_finding_count ?? 0 },
      { header: 'PII Egress', accessor: (r) => r.pii_summary?.egress_finding_count ?? 0 },
      { header: 'Security Labels', accessor: (r) => (r.security_summary?.labels ?? []).join('; ') },
    ], usageData)
  }

  return (
    <div className="flex h-full flex-col" style={{ background: '#F8F9FA' }}>
      <TopBar title="Usage">
        <button
          type="button"
          disabled={usageData.length === 0}
          onClick={exportUsage}
          className="flex items-center gap-1.5 rounded-lg border border-[#E5E7EB] bg-white px-3 py-1.5 text-sm font-medium text-[#374151] hover:bg-[#F9FAFB] disabled:opacity-50"
        >
          <Download size={14} />
          Export CSV
        </button>
      </TopBar>
      <div className="flex-1 overflow-auto p-8">
        <UsageOverviewPanel onDataLoaded={handleDataLoaded} />
      </div>
    </div>
  )
}
