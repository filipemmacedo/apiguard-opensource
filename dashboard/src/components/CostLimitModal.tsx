import { useState } from "react";
import { X, Euro } from "lucide-react";
import { setUserKeyCostLimit, type AdminUserKey } from "#/lib/api";

interface CostLimitModalProps {
  userKey: Pick<AdminUserKey, "id" | "display_name" | "monthly_cost_limit_eur">;
  onClose: () => void;
  onUpdated: () => void;
}

export function CostLimitModal({ userKey, onClose, onUpdated }: CostLimitModalProps) {
  const [value, setValue] = useState(
    userKey.monthly_cost_limit_eur != null ? parseFloat(userKey.monthly_cost_limit_eur.toPrecision(6)).toString() : "",
  );
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    const parsed = parseFloat(value);
    if (!value.trim() || isNaN(parsed) || parsed < 0.0002) {
      setError("Please enter a value of at least €0.0002.");
      return;
    }
    setLoading(true);
    try {
      await setUserKeyCostLimit(userKey.id, parsed);
      onUpdated();
    } catch (err: any) {
      setError(err?.message || "Failed to update cost limit.");
    } finally {
      setLoading(false);
    }
  };

  const handleRemove = async () => {
    setError("");
    setLoading(true);
    try {
      await setUserKeyCostLimit(userKey.id, null);
      onUpdated();
    } catch (err: any) {
      setError(err?.message || "Failed to remove cost limit.");
    } finally {
      setLoading(false);
    }
  };

  const inputClass =
    "h-10 w-full rounded-lg border border-[#D1D5DB] bg-[#F9FAFB] pl-7 pr-3 text-sm text-[#374151] outline-none focus:border-[#00C896] focus:ring-1 focus:ring-[#00C896]";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative mx-4 w-full max-w-md rounded-xl border border-[#E5E7EB] bg-white p-6 shadow-xl">
        <div className="mb-5 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Euro className="h-5 w-5 text-[#00C896]" />
            <h2 className="text-base font-semibold text-[#111827]">Set Monthly Budget</h2>
          </div>
          <button onClick={onClose} className="text-[#9CA3AF] hover:text-[#374151]">
            <X className="h-5 w-5" />
          </button>
        </div>

        <p className="mb-4 text-sm text-[#6B7280]">
          Set a monthly spending cap for{" "}
          <span className="font-medium text-[#111827]">{userKey.display_name}</span>. Requests will
          be blocked once the estimated cost reaches this limit.
        </p>

        {userKey.monthly_cost_limit_eur != null && (
          <div className="mb-4 rounded-lg border border-[#A7F3D0] bg-[#ECFDF5] p-3 text-sm text-[#065F46]">
            Current limit:{" "}
            <span className="font-semibold">€{parseFloat(userKey.monthly_cost_limit_eur.toPrecision(6))} / month</span>
          </div>
        )}

        {error && (
          <div className="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-[#6B7280]">
              Monthly Budget (EUR)
            </label>
            <div className="relative">
              <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-[#9CA3AF]">€</span>
              <input
                type="number"
                min="0.0002"
                step="0.0001"
                value={value}
                onChange={(e) => setValue(e.target.value)}
                placeholder="e.g. 0.0002"
                className={inputClass}
              />
            </div>
          </div>

          <div className="flex items-center justify-between gap-3 pt-1">
            {userKey.monthly_cost_limit_eur != null ? (
              <button
                type="button"
                onClick={handleRemove}
                disabled={loading}
                className="rounded-lg border border-red-200 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50 disabled:opacity-50"
              >
                Remove Limit
              </button>
            ) : (
              <button
                type="button"
                onClick={onClose}
                className="rounded-lg border border-[#E5E7EB] bg-white px-4 py-2 text-sm font-medium text-[#374151] hover:bg-[#F9FAFB]"
              >
                Cancel
              </button>
            )}
            <div className="flex gap-2">
              {userKey.monthly_cost_limit_eur != null && (
                <button
                  type="button"
                  onClick={onClose}
                  className="rounded-lg border border-[#E5E7EB] bg-white px-4 py-2 text-sm font-medium text-[#374151] hover:bg-[#F9FAFB]"
                >
                  Cancel
                </button>
              )}
              <button
                type="submit"
                disabled={loading}
                className="rounded-lg bg-[#00C896] px-4 py-2 text-sm font-medium text-white hover:bg-[#00B085] disabled:opacity-50"
              >
                {loading ? "Saving..." : "Save Limit"}
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
}
