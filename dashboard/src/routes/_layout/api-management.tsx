import { createFileRoute } from '@tanstack/react-router'
import { RefreshCw } from 'lucide-react'
import APIManagementPanel from '#/components/APIManagementPanel'
import { TopBar } from '#/components/ui/TopBar'

export const Route = createFileRoute('/_layout/api-management')({
  component: APIManagementPage,
})

function APIManagementPage() {
  return (
    <div className="flex h-full flex-col" style={{ background: '#F8F9FA' }}>
      <TopBar title="API Management">
        <button
          type="button"
          className="flex items-center gap-1.5 rounded-lg border border-[#E5E7EB] bg-white px-3 py-1.5 text-sm font-medium text-[#374151] hover:bg-[#F9FAFB]"
          onClick={() => window.location.reload()}
        >
          <RefreshCw size={14} />
          Refresh
        </button>
      </TopBar>
      <div className="flex-1 overflow-auto p-8">
        <APIManagementPanel />
      </div>
    </div>
  )
}
