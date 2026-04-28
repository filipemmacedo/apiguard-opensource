import { useState } from 'react'
import { Plus, X } from 'lucide-react'

type NSFWTerm = { id: number; term: string }

type NSFWBlockedTermsSectionProps = {
  terms: NSFWTerm[]
  saving: boolean
  onAdd: (term: string) => void
  onRemove: (id: number) => void
}

export default function NSFWBlockedTermsSection({
  terms,
  saving,
  onAdd,
  onRemove,
}: NSFWBlockedTermsSectionProps) {
  const [input, setInput] = useState('')

  function handleAdd() {
    const trimmed = input.trim()
    if (!trimmed) return
    onAdd(trimmed)
    setInput('')
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') handleAdd()
  }

  return (
    <section className="rounded-xl border border-[#E5E7EB] bg-white p-6">
      <div className="mb-5">
        <p className="m-0 text-xs font-semibold uppercase tracking-wider text-[#00C896]">NSFW Keyword Guardrail</p>
        <h3 className="mt-2 text-xl font-semibold text-[#111827]">Block explicit words and phrases</h3>
        <p className="mt-2 text-sm text-[#6B7280]">
          These terms are stored as admin policy and checked on ingress before any request reaches the
          upstream model. API Guard stores matched policy IDs only, never raw tenant prompt snippets.
        </p>
      </div>

      {/* Add input */}
      <div className="flex gap-2">
        <input
          className="flex-1 rounded-lg border border-[#D1D5DB] bg-white px-3 py-2 text-sm text-[#374151] outline-none focus:border-[#00C896] focus:ring-1 focus:ring-[#00C896] disabled:opacity-50"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={saving}
          placeholder="Add a word or phrase..."
        />
        <button
          type="button"
          onClick={handleAdd}
          disabled={saving || !input.trim()}
          className="flex items-center gap-1.5 rounded-lg bg-[#00C896] px-4 py-2 text-sm font-semibold text-white hover:bg-[#00B085] disabled:opacity-50 transition-colors"
        >
          <Plus className="h-4 w-4" />
          Add Term
        </button>
      </div>

      {/* List */}
      <div className="mt-4">
        {terms.length === 0 ? (
          <p className="rounded-lg border border-dashed border-[#D1D5DB] bg-[#F9FAFB] p-4 text-sm text-[#6B7280]">
            No blocked terms configured yet.
          </p>
        ) : (
          <ul className="divide-y divide-[#F3F4F6] rounded-lg border border-[#E5E7EB]">
            {terms.map((term) => (
              <li key={term.id} className="flex items-center justify-between px-4 py-3">
                <span className="text-sm text-[#111827]">{term.term}</span>
                <button
                  type="button"
                  onClick={() => onRemove(term.id)}
                  disabled={saving}
                  className="ml-4 rounded-lg p-1.5 text-[#9CA3AF] hover:bg-red-50 hover:text-red-600 disabled:opacity-50 transition-colors"
                  aria-label={`Remove ${term.term}`}
                >
                  <X className="h-4 w-4" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}
