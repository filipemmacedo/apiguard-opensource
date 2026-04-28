import { useState } from 'react'
import { ShieldOff } from 'lucide-react'
import { type AdminQuarantine } from '../lib/api'

function formatExpiry(expiresAt: string): string {
  const date = new Date(expiresAt)
  return date.toLocaleString()
}

function ConfirmUnlockDialog({
  userID,
  onConfirm,
  onCancel,
}: {
  userID: string
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="w-full max-w-sm rounded-2xl bg-white shadow-xl p-6 space-y-4">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-amber-100">
            <ShieldOff className="h-5 w-5 text-amber-600" />
          </div>
          <div>
            <p className="font-semibold text-[#111827]">Unlock Quarantine</p>
            <p className="text-xs text-[#6B7280]">This action is audit-logged.</p>
          </div>
        </div>
        <p className="text-sm text-[#374151]">
          Are you sure you want to unlock quarantine for user <span className="font-mono font-semibold">{userID}</span>? They will be able to make requests immediately.
        </p>
        <div className="flex justify-end gap-3">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-lg border border-[#E5E7EB] px-4 py-2 text-sm font-semibold text-[#374151] hover:bg-[#F9FAFB] transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className="rounded-lg bg-amber-500 px-4 py-2 text-sm font-semibold text-white hover:bg-amber-600 transition-colors"
          >
            Unlock
          </button>
        </div>
      </div>
    </div>
  )
}

export function QuarantineBadge({ expiresAt }: { expiresAt: string }) {
  return (
    <span
      title={`Quarantined until ${formatExpiry(expiresAt)}`}
      className="inline-flex items-center gap-1 rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800"
    >
      <ShieldOff className="h-3 w-3" />
      Quarantined
    </span>
  )
}

export default function QuarantinePanel({
  quarantines,
  unlocking,
  onUnlock,
}: {
  quarantines: AdminQuarantine[]
  unlocking: boolean
  onUnlock: (userID: string) => void
}) {
  const [confirmUserID, setConfirmUserID] = useState<string | null>(null)

  return (
    <>
      {confirmUserID && (
        <ConfirmUnlockDialog
          userID={confirmUserID}
          onConfirm={() => {
            onUnlock(confirmUserID)
            setConfirmUserID(null)
          }}
          onCancel={() => setConfirmUserID(null)}
        />
      )}

      <div className="space-y-4">
        {quarantines.length === 0 ? (
          <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-[#D1D5DB] bg-white py-16 text-center">
            <ShieldOff className="h-8 w-8 text-[#9CA3AF]" />
            <p className="font-medium text-[#374151]">No users currently quarantined</p>
            <p className="text-sm text-[#6B7280]">Quarantined users will appear here when the Rate Limiter guardrail is active.</p>
          </div>
        ) : (
          <div className="rounded-xl border border-[#E5E7EB] overflow-hidden bg-white">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[#E5E7EB] bg-[#F9FAFB]">
                  <th className="px-6 py-3 text-left text-xs font-semibold text-[#6B7280]">User ID</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-[#6B7280]">Quarantined at</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-[#6B7280]">Expires at</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-[#6B7280]">Reason</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-[#6B7280]">Actions</th>
                </tr>
              </thead>
              <tbody>
                {quarantines.map((q, i) => (
                  <tr key={q.user_id} className={i % 2 === 1 ? 'bg-[#FAFAFA]' : 'bg-white'}>
                    <td className="border-b border-[#E5E7EB] px-6 py-3 font-mono text-xs text-[#374151]">{q.user_id}</td>
                    <td className="border-b border-[#E5E7EB] px-6 py-3 text-xs text-[#6B7280]">
                      {new Date(q.locked_at).toLocaleString()}
                    </td>
                    <td className="border-b border-[#E5E7EB] px-6 py-3 text-xs text-[#6B7280]">
                      {formatExpiry(q.expires_at)}
                    </td>
                    <td className="border-b border-[#E5E7EB] px-6 py-3 text-xs text-[#6B7280] max-w-xs truncate">
                      {q.locked_reason || '—'}
                    </td>
                    <td className="border-b border-[#E5E7EB] px-6 py-3">
                      <button
                        type="button"
                        disabled={unlocking}
                        onClick={() => setConfirmUserID(q.user_id)}
                        className="rounded-lg border border-amber-300 px-3 py-1.5 text-xs font-semibold text-amber-700 hover:bg-amber-50 disabled:opacity-40 transition-colors"
                      >
                        Unlock
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  )
}
