import type { AdminPIIPolicy } from '../lib/api'
import { Select } from './ui/Select'

type PIIAction = 'off' | 'observe' | 'block'

type PIIPolicySectionProps = {
  piiPolicies: AdminPIIPolicy[]
  saving: boolean
  onChange: (entityType: string, value: PIIAction) => void
}

const ACTION_OPTIONS = [
  { value: 'off', label: 'Off' },
  { value: 'observe', label: 'Observe' },
  { value: 'block', label: 'Block' },
]

function policyToAction(policy: AdminPIIPolicy): PIIAction {
  if (!policy.enabled) return 'off'
  return policy.action
}

export default function PIIPolicySection({ piiPolicies, saving, onChange }: PIIPolicySectionProps) {
  return (
    <section className="rounded-xl border border-[#E5E7EB] bg-white p-6">
      <div className="mb-5">
        <p className="m-0 text-xs font-semibold uppercase tracking-wider text-[#00C896]">PII Guardrail Policy</p>
        <h3 className="mt-2 text-xl font-semibold text-[#111827]">Manage what counts as PII</h3>
        <p className="mt-2 text-sm text-[#6B7280]">
          These rules drive ingress and egress PII detection in the proxy. Findings are fingerprinted
          and summarised for admins without storing raw matched values.
        </p>
      </div>

      {piiPolicies.length === 0 ? (
        <p className="rounded-lg border border-dashed border-[#D1D5DB] bg-[#F9FAFB] p-4 text-sm text-[#6B7280]">
          No PII policies loaded.
        </p>
      ) : (
        <ul className="divide-y divide-[#F3F4F6] rounded-lg border border-[#E5E7EB]">
          {piiPolicies.map((policy) => (
            <li key={policy.entity_type} className="flex items-center justify-between px-4 py-3 gap-4">
              <div className="min-w-0">
                <p className="text-sm font-medium text-[#111827]">{policy.display_name}</p>
                <p className="text-xs text-[#9CA3AF] uppercase tracking-wide">{policy.entity_type}</p>
              </div>
              <Select
                value={policyToAction(policy)}
                onChange={(val) => onChange(policy.entity_type, val as PIIAction)}
                options={ACTION_OPTIONS}
                disabled={saving}
                className="w-32 shrink-0"
                aria-label={`Action for ${policy.display_name}`}
              />
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
