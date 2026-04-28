import { createFileRoute } from '@tanstack/react-router'
import GuardrailsPanel from '#/components/GuardrailsPanel'
import { TopBar } from '#/components/ui/TopBar'

export const Route = createFileRoute('/_layout/guardrails')({
  component: GuardrailsPage,
})

function GuardrailsPage() {
  return (
    <div className="flex h-full flex-col" style={{ background: '#F8F9FA' }}>
      <TopBar title="Guardrails" />
      <div className="flex-1 overflow-auto p-8">
        <GuardrailsPanel />
      </div>
    </div>
  )
}
