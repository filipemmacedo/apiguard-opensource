import { useEffect, useState } from "react";
import {
  APIError,
  createAdminProvider,
  createAdminUserKey,
  deleteAdminProvider,
  deleteAdminUserKey,
  fetchAdminProviderModels,
  fetchAdminProviders,
  fetchAdminUserKeys,
  listQuarantines,
  refreshAdminProviderModels,
  rotateAdminProvider,
  unlockQuarantine,
  updateAdminProviderModels,
  type AdminProviderCredential,
  type AdminProviderModel,
  type AdminQuarantine,
  type AdminUserKey,
} from "../lib/api";
import { CostLimitModal } from "./CostLimitModal";
import { PasswordInput } from "#/components/ui/PasswordInput";
import { TabBar } from "./ui/TabBar";
import { Select } from "./ui/Select";
import QuarantinePanel, { QuarantineBadge } from "./QuarantinePanel";

type DraftStatus = "idle" | "loading" | "saving";

export default function APIManagementPanel() {
  const [activeTab, setActiveTab] = useState("user-keys");
  const [userKeys, setUserKeys] = useState<AdminUserKey[]>([]);
  const [costLimitKey, setCostLimitKey] = useState<AdminUserKey | null>(null);
  const [providers, setProviders] = useState<AdminProviderCredential[]>([]);
  const [allProviderModels, setAllProviderModels] = useState<
    Record<number, AdminProviderModel[]>
  >({});
  const [selectedProviderID, setSelectedProviderID] = useState<number | null>(
    null,
  );
  const [expandedProviders, setExpandedProviders] = useState<Set<number>>(
    new Set(),
  );
  const [catalogEnabledModelIDs, setCatalogEnabledModelIDs] = useState<
    Record<number, string[]>
  >({});
  const [quarantines, setQuarantines] = useState<AdminQuarantine[]>([]);
  const [quarantineUnlocking, setQuarantineUnlocking] = useState(false);
  const [loadStatus, setLoadStatus] = useState<DraftStatus>("idle");
  const [userStatus, setUserStatus] = useState<DraftStatus>("idle");
  const [providerStatus, setProviderStatus] = useState<DraftStatus>("idle");
  const [modelsStatus, setModelsStatus] = useState<DraftStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [providerType, setProviderType] = useState("openai");
  const [revealedUserKey, setRevealedUserKey] = useState<string | null>(null);
  const [userDisplayName, setUserDisplayName] = useState("");
  const [providerDisplayName, setProviderDisplayName] = useState("OpenAI");
  const [providerBaseURL, setProviderBaseURL] = useState(
    "https://api.openai.com",
  );
  const [providerAPIKey, setProviderAPIKey] = useState("");
  const [editingProvider, setEditingProvider] =
    useState<AdminProviderCredential | null>(null);
  const [editAPIKey, setEditAPIKey] = useState("");
  const [deletingProvider, setDeletingProvider] =
    useState<AdminProviderCredential | null>(null);

  const providerDefaults: Record<
    string,
    { displayName: string; baseURL: string; placeholder: string }
  > = {
    openai: {
      displayName: "OpenAI",
      baseURL: "https://api.openai.com",
      placeholder: "sk-...",
    },
    anthropic: {
      displayName: "Anthropic",
      baseURL: "https://api.anthropic.com",
      placeholder: "sk-ant-...",
    },
    google: {
      displayName: "Google",
      baseURL: "https://generativelanguage.googleapis.com",
      placeholder: "AIza...",
    },
  };

  function onProviderTypeChange(type: string) {
    setProviderType(type);
    const defaults = providerDefaults[type];
    if (defaults) {
      setProviderDisplayName(defaults.displayName);
      setProviderBaseURL(defaults.baseURL);
    }
  }

  useEffect(() => {
    void loadManagementData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function loadManagementData(preferredProviderID?: number) {
    setLoadStatus("loading");
    setError(null);
    setSuccessMessage(null);
    try {
      const [userPayload, providerPayload, quarantinePayload] = await Promise.all([
        fetchAdminUserKeys(),
        fetchAdminProviders(),
        listQuarantines().catch(() => ({ quarantines: [] as AdminQuarantine[] })),
      ]);
      setUserKeys(userPayload.user_keys);
      setProviders(providerPayload.providers);
      setQuarantines(quarantinePayload.quarantines);

      const nextProvider =
        providerPayload.providers.find(
          (provider) => provider.id === preferredProviderID,
        ) ??
        providerPayload.providers[0] ??
        null;
      setSelectedProviderID(nextProvider?.id ?? null);

      // Fetch models for all providers (for Master Catalog)
      const modelsMap: Record<number, AdminProviderModel[]> = {};
      const enabledMap: Record<number, string[]> = {};
      await Promise.all(
        providerPayload.providers.map(async (p) => {
          try {
            const payload = await fetchAdminProviderModels(p.id);
            modelsMap[p.id] = payload.models;
            enabledMap[p.id] = payload.models
              .filter((m) => m.enabled && m.sync_state === "synced")
              .map((m) => m.provider_model_id);
          } catch {
            modelsMap[p.id] = [];
            enabledMap[p.id] = [];
          }
        }),
      );
      setAllProviderModels(modelsMap);
      setCatalogEnabledModelIDs(enabledMap);

      // Default: all collapsed
      setExpandedProviders(new Set());
    } catch (err) {
      handleAdminError(err, "Failed to load API Management data.");
    } finally {
      setLoadStatus("idle");
    }
  }

  function handleAdminError(err: unknown, fallbackMessage: string) {
    if (err instanceof APIError) {
      setError(err.message);
      return;
    }
    setError(err instanceof Error ? err.message : fallbackMessage);
  }

  async function onCreateUserKey(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!userDisplayName.trim()) {
      setError("Display name is required.");
      return;
    }

    setUserStatus("saving");
    setError(null);
    setSuccessMessage(null);
    setRevealedUserKey(null);
    try {
      const payload = await createAdminUserKey({
        displayName: userDisplayName,
      });
      setUserKeys((current) => [payload.user_key, ...current]);
      setRevealedUserKey(payload.raw_api_key);
      setUserDisplayName("");
      setSuccessMessage(
        "User API key created. Copy the full key now; it will not be shown again.",
      );
    } catch (err) {
      handleAdminError(err, "Failed to create user key.");
    } finally {
      setUserStatus("idle");
    }
  }

  async function onDeleteUserKey(id: number) {
    setUserStatus("saving");
    setError(null);
    setSuccessMessage(null);
    try {
      await deleteAdminUserKey(id);
      await loadManagementData(selectedProviderID ?? undefined);
      setSuccessMessage("User API key deleted.");
    } catch (err) {
      handleAdminError(err, "Failed to delete user key.");
    } finally {
      setUserStatus("idle");
    }
  }

  async function onCreateProvider(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!providerAPIKey.trim()) {
      setError("Provider API key is required.");
      return;
    }
    const displayName =
      providerDisplayName ||
      providerDefaults[providerType]?.displayName ||
      providerType;
    const duplicate = providers.find(
      (p) => p.provider_type === providerType && p.display_name === displayName,
    );
    if (duplicate) {
      setError(
        `A ${providerDefaults[providerType]?.displayName ?? providerType} provider named "${displayName}" already exists.`,
      );
      return;
    }

    setProviderStatus("saving");
    setError(null);
    setSuccessMessage(null);
    try {
      const payload = await createAdminProvider({
        providerType,
        displayName:
          providerDisplayName ||
          providerDefaults[providerType]?.displayName ||
          providerType,
        apiKey: providerAPIKey,
        baseURL: providerBaseURL || undefined,
      });
      setProviders((current) => [...current, payload.provider]);
      setSelectedProviderID(payload.provider.id);
      setAllProviderModels((current) => ({
        ...current,
        [payload.provider.id]: payload.models,
      }));
      setCatalogEnabledModelIDs((current) => ({
        ...current,
        [payload.provider.id]: payload.models
          .filter((m) => m.enabled && m.sync_state === "synced")
          .map((m) => m.provider_model_id),
      }));
      setExpandedProviders(
        (current) => new Set([...current, payload.provider.id]),
      );
      setProviderAPIKey("");
      setSuccessMessage(
        "Provider API key validated and stored. Models were synced server-side.",
      );
    } catch (err) {
      handleAdminError(err, "Failed to create provider.");
    } finally {
      setProviderStatus("idle");
    }
  }

  async function onConfirmDeleteProvider() {
    if (!deletingProvider) return;
    const id = deletingProvider.id;
    setDeletingProvider(null);
    setProviderStatus("saving");
    setError(null);
    setSuccessMessage(null);
    try {
      await deleteAdminProvider(id);
      setProviders((current) => current.filter((p) => p.id !== id));
      if (selectedProviderID === id) {
        setSelectedProviderID(null);
      }
      setAllProviderModels((current) => {
        const next = { ...current };
        delete next[id];
        return next;
      });
      setCatalogEnabledModelIDs((current) => {
        const next = { ...current };
        delete next[id];
        return next;
      });
      setSuccessMessage("Provider credential deleted.");
    } catch (err) {
      handleAdminError(err, "Failed to delete provider.");
    } finally {
      setProviderStatus("idle");
    }
  }

  async function onConfirmEditProvider() {
    if (!editingProvider) return;
    if (!editAPIKey.trim()) {
      setError("A new API key is required to update the provider.");
      return;
    }
    const id = editingProvider.id;
    setEditingProvider(null);
    setProviderStatus("saving");
    setError(null);
    setSuccessMessage(null);
    try {
      const payload = await rotateAdminProvider(id, editAPIKey);
      setProviders((current) =>
        current.map((p) =>
          p.id === payload.provider.id ? payload.provider : p,
        ),
      );
      setAllProviderModels((current) => ({
        ...current,
        [payload.provider.id]: payload.models,
      }));
      setCatalogEnabledModelIDs((current) => ({
        ...current,
        [payload.provider.id]: payload.models
          .filter((m) => m.enabled && m.sync_state === "synced")
          .map((m) => m.provider_model_id),
      }));
      setEditAPIKey("");
      setSuccessMessage("Provider API key updated and models re-synced.");
    } catch (err) {
      handleAdminError(err, "Failed to update provider.");
    } finally {
      setProviderStatus("idle");
    }
  }

  function toggleCatalogModel(providerID: number, modelID: string) {
    setCatalogEnabledModelIDs((current) => {
      const ids = current[providerID] ?? [];
      return {
        ...current,
        [providerID]: ids.includes(modelID)
          ? ids.filter((v) => v !== modelID)
          : [...ids, modelID].sort(),
      };
    });
  }

  function toggleAllCatalogModels(providerID: number) {
    const models = allProviderModels[providerID] ?? [];
    const eligibleModels = models.filter((m) => m.sync_state === "synced");
    const currentEnabled = catalogEnabledModelIDs[providerID] ?? [];
    const allEnabled = eligibleModels.every((m) =>
      currentEnabled.includes(m.provider_model_id),
    );
    setCatalogEnabledModelIDs((current) => ({
      ...current,
      [providerID]: allEnabled
        ? []
        : eligibleModels.map((m) => m.provider_model_id).sort(),
    }));
  }

  function toggleExpandProvider(providerID: number) {
    setExpandedProviders((current) => {
      const next = new Set(current);
      if (next.has(providerID)) {
        next.delete(providerID);
      } else {
        next.add(providerID);
      }
      return next;
    });
  }

  async function onRefreshProviderModels(providerID: number) {
    setModelsStatus("saving");
    setError(null);
    setSuccessMessage(null);
    try {
      const payload = await refreshAdminProviderModels(providerID);
      setProviders((current) =>
        current.map((p) =>
          p.id === payload.provider.id ? payload.provider : p,
        ),
      );
      setAllProviderModels((current) => ({
        ...current,
        [providerID]: payload.models,
      }));
      setCatalogEnabledModelIDs((current) => ({
        ...current,
        [providerID]: payload.models
          .filter((m) => m.enabled && m.sync_state === "synced")
          .map((m) => m.provider_model_id),
      }));
      setSuccessMessage("Provider model list refreshed.");
    } catch (err) {
      handleAdminError(err, "Failed to refresh provider models.");
    } finally {
      setModelsStatus("idle");
    }
  }

  async function onSaveAllCatalogModels() {
    setModelsStatus("saving");
    setError(null);
    setSuccessMessage(null);
    try {
      for (const provider of providers) {
        const enabledIDs = catalogEnabledModelIDs[provider.id] ?? [];
        const payload = await updateAdminProviderModels(
          provider.id,
          enabledIDs,
        );
        setProviders((current) =>
          current.map((p) =>
            p.id === payload.provider.id ? payload.provider : p,
          ),
        );
        setAllProviderModels((current) => ({
          ...current,
          [provider.id]: payload.models,
        }));
      }
      setSuccessMessage("Enabled model list updated.");
    } catch (err) {
      handleAdminError(err, "Failed to update enabled models.");
    } finally {
      setModelsStatus("idle");
    }
  }

  async function onUnlockQuarantine(userID: string) {
    setQuarantineUnlocking(true);
    setError(null);
    try {
      await unlockQuarantine(userID);
      setQuarantines((current) => current.filter((q) => q.user_id !== userID));
      setSuccessMessage(`Quarantine lifted for user ${userID}.`);
    } catch (err) {
      handleAdminError(err, "Failed to unlock quarantine.");
    } finally {
      setQuarantineUnlocking(false);
    }
  }

  const quarantinedUserIDs = new Set(quarantines.map((q) => q.user_id));

  const tabs = [
    { id: "user-keys", label: "User Keys", count: userKeys.length },
    { id: "provider-keys", label: "LLM Configuration", count: providers.length },
    {
      id: "master-catalog",
      label: "Master Catalog",
      count: Object.values(allProviderModels)
        .flat()
        .filter((m) => m.enabled).length,
    },
    { id: "quarantine", label: "Quarantine", count: quarantines.length, countVariant: "warning" as const },
  ];

  const inputClass =
    "h-10 w-full rounded-lg border border-[#D1D5DB] bg-[#F9FAFB] px-3 text-sm text-[#374151] outline-none focus:border-[#00C896] focus:ring-1 focus:ring-[#00C896]";
  const labelClass = "text-xs font-medium text-[#6B7280]";
  const btnPrimary =
    "rounded-lg bg-[#00C896] px-4 py-2.5 text-sm font-medium text-white hover:bg-[#00B085] disabled:opacity-60";
  const btnOutline =
    "rounded-lg border border-[#E5E7EB] bg-white px-3 py-1.5 text-xs font-medium text-[#374151] hover:bg-[#F9FAFB] disabled:opacity-40";

  return (
    <>
    <div className="flex flex-col gap-5">
      <TabBar tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab} />

      {error && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}
      {successMessage && (
        <div className="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">
          {successMessage}
        </div>
      )}

      {loadStatus === "loading" && userKeys.length === 0 && (
        <div className="space-y-4">
          <div className="rounded-xl border border-[#E5E7EB] bg-white p-6">
            <div className="h-5 w-48 animate-pulse rounded bg-[#E5E7EB]" />
            <div className="mt-3 h-3 w-64 animate-pulse rounded bg-[#F3F4F6]" />
            <div className="mt-6 flex flex-col gap-3 sm:flex-row">
              <div className="h-10 flex-1 animate-pulse rounded-lg bg-[#F3F4F6]" />
              <div className="h-10 flex-1 animate-pulse rounded-lg bg-[#F3F4F6]" />
              <div className="h-10 w-40 animate-pulse rounded-lg bg-[#E5E7EB]" />
            </div>
          </div>
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="flex gap-6 rounded-xl border border-[#E5E7EB] bg-white px-6 py-4"
            >
              <div className="h-4 w-24 animate-pulse rounded bg-[#E5E7EB]" />
              <div className="h-4 w-32 animate-pulse rounded bg-[#F3F4F6]" />
              <div className="h-4 w-16 animate-pulse rounded bg-[#F3F4F6]" />
            </div>
          ))}
        </div>
      )}

      {revealedUserKey && (
        <div className="rounded-xl border border-[#E5E7EB] bg-white p-5">
          <p className="mb-1 text-sm font-semibold text-[#111827]">
            One-time user API key reveal
          </p>
          <p className="mb-3 text-xs text-[#6B7280]">
            This is the only time the full user API key is returned by the
            server.
          </p>
          <pre className="overflow-auto rounded-lg bg-[#0A0F1C] p-3 text-xs text-[#00C896]">
            {revealedUserKey}
          </pre>
        </div>
      )}

      {/* User Keys Tab */}
      {activeTab === "user-keys" && (
        <div className="space-y-5">
          <div className="rounded-xl border border-[#E5E7EB] bg-white p-6">
            <div className="mb-1 flex items-center justify-between">
              <h3
                className="text-sm font-semibold text-[#111827]"
                style={{ fontFamily: "'Space Grotesk', sans-serif" }}
              >
                Provision user credentials
              </h3>
              <span className="rounded-full bg-[#F3F4F6] px-2.5 py-0.5 text-xs font-medium text-[#6B7280]">
                {userKeys.length} created
              </span>
            </div>
            <p className="mb-5 text-xs text-[#9CA3AF]">
              Generate API keys for users to authenticate proxy requests. User
              IDs are assigned automatically.
            </p>

            <form
              className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end"
              onSubmit={onCreateUserKey}
            >
              <label className="flex flex-1 min-w-[200px] flex-col gap-1.5">
                <span className={labelClass}>Display Name</span>
                <input
                  className={inputClass}
                  value={userDisplayName}
                  onChange={(e) => setUserDisplayName(e.target.value)}
                  placeholder="Acme Production"
                />
              </label>
              <button
                type="submit"
                className={btnPrimary}
                disabled={userStatus !== "idle"}
              >
                {userStatus === "saving"
                  ? "Creating..."
                  : "Create User API Key"}
              </button>
            </form>
          </div>

          {/* Desktop table */}
          <div className="hidden overflow-hidden rounded-xl border border-[#E5E7EB] bg-white md:block">
            <table className="w-full">
              <thead>
                <tr className="bg-[#F9FAFB]">
                  <th
                    scope="col"
                    className="border-b border-[#E5E7EB] px-6 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-[#6B7280]"
                  >
                    User
                  </th>
                  <th
                    scope="col"
                    className="border-b border-[#E5E7EB] px-6 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-[#6B7280]"
                  >
                    Masked Key
                  </th>
                  <th
                    scope="col"
                    className="border-b border-[#E5E7EB] px-6 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-[#6B7280]"
                  >
                    Status
                  </th>
                  <th
                    scope="col"
                    className="border-b border-[#E5E7EB] px-6 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-[#6B7280]"
                  >
                    Last Used
                  </th>
                  <th
                    scope="col"
                    className="border-b border-[#E5E7EB] px-6 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-[#6B7280]"
                  >
                    Cost Limit
                  </th>
                  <th
                    scope="col"
                    className="border-b border-[#E5E7EB] px-6 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-[#6B7280]"
                  >
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {userKeys.length === 0 ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-6 py-10 text-center text-sm text-[#9CA3AF]"
                    >
                      No user keys have been created yet.
                    </td>
                  </tr>
                ) : (
                  userKeys.map((tk, i) => (
                    <tr
                      key={tk.id}
                      className={i % 2 === 1 ? "bg-[#FAFAFA]" : "bg-white"}
                    >
                      <td className="border-b border-[#E5E7EB] px-6 py-3">
                        <div className="text-sm font-medium text-[#111827]">
                          {tk.display_name}
                        </div>
                        <div className="flex items-center gap-2 flex-wrap mt-0.5">
                          <span className="text-xs text-[#9CA3AF]">{tk.user_id}</span>
                          {quarantinedUserIDs.has(tk.user_id) && (
                            <QuarantineBadge
                              expiresAt={quarantines.find((q) => q.user_id === tk.user_id)!.expires_at}
                            />
                          )}
                        </div>
                      </td>
                      <td className="border-b border-[#E5E7EB] px-6 py-3 font-mono text-xs text-[#374151]">
                        {tk.masked_key}
                      </td>
                      <td className="border-b border-[#E5E7EB] px-6 py-3">
                        <span
                          className={`inline-flex rounded-md px-2 py-0.5 text-xs font-medium ${
                            tk.status === "active"
                              ? "bg-[#ECFDF5] text-[#059669]"
                              : "bg-[#FEF2F2] text-[#DC2626]"
                          }`}
                        >
                          {tk.status}
                        </span>
                      </td>
                      <td className="border-b border-[#E5E7EB] px-6 py-3 text-xs text-[#6B7280]">
                        {tk.last_used_at
                          ? new Date(tk.last_used_at).toLocaleString()
                          : "Never"}
                      </td>
                      <td className="border-b border-[#E5E7EB] px-6 py-3 text-xs tabular-nums">
                        {tk.monthly_cost_limit_eur != null ? (
                          <span className="font-medium text-[#111827]">
                            €{parseFloat(tk.monthly_cost_limit_eur.toPrecision(6))}/mo
                          </span>
                        ) : (
                          <span className="text-[#9CA3AF]">No limit</span>
                        )}
                      </td>
                      <td className="border-b border-[#E5E7EB] px-6 py-3">
                        <div className="flex gap-2">
                          <button
                            type="button"
                            className="rounded-lg border border-[#E5E7EB] bg-white px-3 py-1.5 text-xs font-medium text-[#374151] hover:bg-[#F9FAFB] disabled:opacity-40"
                            onClick={() => setCostLimitKey(tk)}
                            disabled={userStatus !== "idle"}
                          >
                            Monthly Budget
                          </button>
                          <button
                            type="button"
                            className="rounded-lg border border-red-200 bg-white px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-50 disabled:opacity-40"
                            onClick={() => void onDeleteUserKey(tk.id)}
                            disabled={userStatus !== "idle"}
                          >
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {/* Mobile card view */}
          <div className="space-y-3 md:hidden">
            {userKeys.length === 0 ? (
              <div className="rounded-xl border border-[#E5E7EB] bg-white p-6 text-center text-sm text-[#9CA3AF]">
                No user keys have been created yet.
              </div>
            ) : (
              userKeys.map((tk) => (
                <div
                  key={tk.id}
                  className="rounded-lg border border-[#E5E7EB] bg-white p-4"
                >
                  <div className="flex items-start justify-between gap-2">
                    <div>
                      <div className="text-sm font-medium text-[#111827]">
                        {tk.display_name}
                      </div>
                      <div className="text-xs text-[#9CA3AF]">{tk.user_id}</div>
                    </div>
                    <span
                      className={`shrink-0 rounded-md px-2 py-0.5 text-xs font-medium ${
                        tk.status === "active"
                          ? "bg-[#ECFDF5] text-[#059669]"
                          : "bg-[#FEF2F2] text-[#DC2626]"
                      }`}
                    >
                      {tk.status}
                    </span>
                  </div>
                  <div className="mt-2 font-mono text-xs text-[#374151]">
                    {tk.masked_key}
                  </div>
                  <div className="mt-1 text-xs text-[#6B7280]">
                    Last used:{" "}
                    {tk.last_used_at
                      ? new Date(tk.last_used_at).toLocaleString()
                      : "Never"}
                  </div>
                  <div className="mt-1 text-xs text-[#6B7280]">
                    Cost limit:{" "}
                    {tk.monthly_cost_limit_eur != null
                      ? <span className="font-medium text-[#111827]">€{parseFloat(tk.monthly_cost_limit_eur.toPrecision(6))}/mo</span>
                      : "No limit"}
                  </div>
                  <div className="mt-3 flex gap-2">
                    <button
                      type="button"
                      className="rounded-lg border border-[#E5E7EB] bg-white px-3 py-1.5 text-xs font-medium text-[#374151] hover:bg-[#F9FAFB] disabled:opacity-40"
                      onClick={() => setCostLimitKey(tk)}
                      disabled={userStatus !== "idle"}
                    >
                      Monthly Budget
                    </button>
                    <button
                      type="button"
                      className="rounded-lg border border-red-200 bg-white px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-50 disabled:opacity-40"
                      onClick={() => void onDeleteUserKey(tk.id)}
                      disabled={userStatus !== "idle"}
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Provider Keys Tab */}
      {activeTab === "provider-keys" && (
        <div className="space-y-5">
          <div className="rounded-xl border border-[#E5E7EB] bg-white p-6">
            <div className="mb-1 flex items-center justify-between">
              <h3
                className="text-sm font-semibold text-[#111827]"
                style={{ fontFamily: "'Space Grotesk', sans-serif" }}
              >
                Store upstream credentials
              </h3>
              <span className="rounded-full bg-[#F3F4F6] px-2.5 py-0.5 text-xs font-medium text-[#6B7280]">
                {providers.length === 0
                  ? "No provider"
                  : `${providers.length} configured`}
              </span>
            </div>
            <p className="mb-5 text-xs text-[#9CA3AF]">
              Provider keys are validated, encrypted at rest, and only shown as
              masked previews.
            </p>

            <form className="space-y-4" onSubmit={onCreateProvider}>
              <div className="grid gap-4 md:grid-cols-2">
                <div className="flex flex-col gap-1.5">
                  <span className={labelClass}>Provider</span>
                  <Select
                    value={providerType}
                    onChange={onProviderTypeChange}
                    options={[
                      {
                        value: "openai",
                        label: "OpenAI",
                        description: "GPT models",
                      },
                      {
                        value: "anthropic",
                        label: "Anthropic",
                        description: "Claude models",
                      },
                      {
                        value: "google",
                        label: "Google",
                        description: "Gemini models",
                      },
                    ]}
                  />
                </div>
                <label className="flex flex-col gap-1.5">
                  <span className={labelClass}>Display Name</span>
                  <input
                    className={inputClass}
                    value={providerDisplayName}
                    onChange={(e) => setProviderDisplayName(e.target.value)}
                  />
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className={labelClass}>Base URL</span>
                  <input
                    className={inputClass}
                    value={providerBaseURL}
                    onChange={(e) => setProviderBaseURL(e.target.value)}
                    placeholder={providerDefaults[providerType]?.baseURL ?? ""}
                  />
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className={labelClass}>API Key</span>
                  <PasswordInput
                    className={inputClass}
                    value={providerAPIKey}
                    onChange={(e) => setProviderAPIKey(e.target.value)}
                    placeholder={
                      providerDefaults[providerType]?.placeholder ?? "API key"
                    }
                  />
                </label>
              </div>
              <button
                type="submit"
                className={`mb-4 ${btnPrimary}`}
                disabled={providerStatus !== "idle"}
              >
                {providerStatus === "saving"
                  ? "Validating..."
                  : "Add Provider API Key"}
              </button>
            </form>

            {providers.length > 0 && (
              <div className="space-y-3">
                {providers.map((provider) => (
                  <div
                    key={provider.id}
                    className="rounded-xl border border-[#E5E7EB] p-4"
                  >
                    <div className="flex items-start justify-between">
                      <div>
                        <div className="flex items-center gap-2">
                          <div className="text-sm font-medium text-[#111827]">
                            {provider.display_name}
                          </div>
                          <span className="rounded-md bg-[#F3F4F6] px-1.5 py-0.5 text-[10px] font-medium uppercase text-[#6B7280]">
                            {provider.provider_type}
                          </span>
                          <span
                            className={`rounded-md px-2 py-0.5 text-xs font-medium ${
                              provider.status === "active"
                                ? "bg-[#ECFDF5] text-[#059669]"
                                : "bg-[#FEF2F2] text-[#DC2626]"
                            }`}
                          >
                            {provider.status}
                          </span>
                        </div>
                        <div className="mt-1 text-xs text-[#9CA3AF]">
                          {provider.masked_key}
                        </div>
                        <div className="mt-1 text-xs text-[#6B7280]">
                          {provider.base_url}
                        </div>
                        {provider.last_validation_error ? (
                          <div className="mt-1 text-xs text-red-600">
                            {provider.last_validation_error}
                          </div>
                        ) : (
                          <div className="mt-1 text-xs text-[#9CA3AF]">
                            Last validated:{" "}
                            {provider.last_validated_at
                              ? new Date(
                                  provider.last_validated_at,
                                ).toLocaleString()
                              : "Never"}
                          </div>
                        )}
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          type="button"
                          className={btnOutline}
                          onClick={() => {
                            setEditingProvider(provider);
                            setEditAPIKey("");
                          }}
                          disabled={providerStatus !== "idle"}
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          className="rounded-lg border border-red-200 bg-white px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-50 disabled:opacity-40"
                          onClick={() => setDeletingProvider(provider)}
                          disabled={providerStatus !== "idle"}
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Master Catalog Tab */}
      {activeTab === "master-catalog" && (
        <div className="space-y-4">
          <div className="rounded-xl border border-[#E5E7EB] bg-white p-6">
            <div className="mb-1 flex items-center justify-between">
              <h3
                className="text-sm font-semibold text-[#111827]"
                style={{ fontFamily: "'Space Grotesk', sans-serif" }}
              >
                Enable synced models
              </h3>
            </div>
            <p className="mb-5 text-xs text-[#9CA3AF]">
              Toggle which provider models are available to tenants at request
              time.
            </p>

            {providers.length === 0 ? (
              <p className="py-8 text-center text-sm text-[#9CA3AF]">
                Add a provider API key to sync the available model catalog.
              </p>
            ) : (
              <>
                <div className="space-y-3">
                  {providers.map((provider) => {
                    const models = allProviderModels[provider.id] ?? [];
                    const enabledIDs =
                      catalogEnabledModelIDs[provider.id] ?? [];
                    const eligibleModels = models.filter(
                      (m) => m.sync_state === "synced",
                    );
                    const allEnabled =
                      eligibleModels.length > 0 &&
                      eligibleModels.every((m) =>
                        enabledIDs.includes(m.provider_model_id),
                      );
                    const enabledEligibleCount = eligibleModels.filter((m) =>
                      enabledIDs.includes(m.provider_model_id),
                    ).length;
                    const isExpanded = expandedProviders.has(provider.id);

                    return (
                      <div
                        key={provider.id}
                        className="rounded-xl border border-[#E5E7EB] overflow-hidden"
                      >
                        {/* Accordion Header */}
                        <div className="flex items-center justify-between bg-[#F9FAFB] px-4 py-3">
                          <button
                            type="button"
                            className="flex flex-1 items-center gap-2 text-left"
                            onClick={() => toggleExpandProvider(provider.id)}
                          >
                            <svg
                              className={`h-4 w-4 text-[#6B7280] transition-transform ${isExpanded ? "rotate-90" : ""}`}
                              fill="none"
                              viewBox="0 0 24 24"
                              stroke="currentColor"
                              strokeWidth={2}
                            >
                              <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                d="M9 5l7 7-7 7"
                              />
                            </svg>
                            <span className="text-sm font-semibold text-[#111827]">
                              {provider.display_name}
                            </span>
                            <span className="rounded-md bg-[#F3F4F6] px-1.5 py-0.5 text-[10px] font-medium uppercase text-[#6B7280]">
                              {provider.provider_type}
                            </span>
                            <span className="rounded-full bg-[#E5E7EB] px-2 py-0.5 text-[10px] font-medium text-[#6B7280]">
                              {enabledEligibleCount}/{eligibleModels.length} models
                            </span>
                          </button>
                          <div className="flex items-center gap-3">
                            {/* Toggle All */}
                            <label className="flex items-center gap-1.5 text-xs text-[#6B7280]">
                              <span>All</span>
                              <div className="relative">
                                <input
                                  type="checkbox"
                                  className="sr-only peer"
                                  checked={allEnabled}
                                  onChange={() =>
                                    toggleAllCatalogModels(provider.id)
                                  }
                                  disabled={
                                    eligibleModels.length === 0 ||
                                    modelsStatus !== "idle"
                                  }
                                />
                                <div className="h-5 w-9 rounded-full bg-[#E5E7EB] peer-checked:bg-[#00C896] peer-disabled:opacity-40 transition-colors" />
                                <div className="absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform peer-checked:translate-x-4" />
                              </div>
                            </label>
                            {/* Refresh */}
                            <button
                              type="button"
                              className={btnOutline}
                              onClick={() =>
                                void onRefreshProviderModels(provider.id)
                              }
                              disabled={modelsStatus !== "idle"}
                            >
                              Refresh Models
                            </button>
                          </div>
                        </div>

                        {/* Accordion Body */}
                        {isExpanded && (
                          <div
                            className="divide-y divide-[#F3F4F6] overflow-y-auto"
                            style={{ maxHeight: 250 }}
                          >
                            {models.length === 0 ? (
                              <p className="px-4 py-6 text-center text-sm text-[#9CA3AF]">
                                No models available. Click Refresh Models to
                                sync.
                              </p>
                            ) : (
                              models.map((model) => {
                                const isEligible = model.sync_state === "synced";
                                return (
                                  <div
                                    key={model.id}
                                    className="flex items-center justify-between px-4 py-2.5 hover:bg-[#F9FAFB]"
                                  >
                                    <div>
                                      <div className="flex items-center gap-2">
                                        <span className="text-sm font-medium text-[#111827]">
                                          {model.display_name}
                                        </span>
                                      </div>
                                      <div className="text-xs text-[#9CA3AF]">
                                        {model.provider_model_id} &middot;{" "}
                                        {model.sync_state}
                                      </div>
                                    </div>
                                    <label className="relative cursor-pointer">
                                      <input
                                        type="checkbox"
                                        className="sr-only peer"
                                        checked={enabledIDs.includes(
                                          model.provider_model_id,
                                        )}
                                        onChange={() =>
                                          toggleCatalogModel(
                                            provider.id,
                                            model.provider_model_id,
                                          )
                                        }
                                        disabled={!isEligible || modelsStatus !== "idle"}
                                      />
                                      <div className="h-6 w-11 rounded-full bg-[#E5E7EB] peer-checked:bg-[#00C896] peer-disabled:opacity-40 transition-colors" />
                                      <div className="absolute left-0.5 top-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform peer-checked:translate-x-5" />
                                    </label>
                                  </div>
                                );
                              })
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>

                <button
                  type="button"
                  className={`mt-4 ${btnPrimary}`}
                  onClick={() => void onSaveAllCatalogModels()}
                  disabled={modelsStatus !== "idle"}
                >
                  {modelsStatus === "saving"
                    ? "Saving..."
                    : "Save Enabled Models"}
                </button>
              </>
            )}
          </div>
        </div>
      )}

      {/* Quarantine Tab */}
      {activeTab === "quarantine" && (
        <QuarantinePanel
          quarantines={quarantines}
          unlocking={quarantineUnlocking}
          onUnlock={(userID) => void onUnlockQuarantine(userID)}
        />
      )}

      {/* Edit Provider Modal */}
      {editingProvider && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
          onClick={() => setEditingProvider(null)}
        >
          <div
            className="w-full max-w-md rounded-xl bg-white p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h3
              className="text-base font-semibold text-[#111827]"
              style={{ fontFamily: "'Space Grotesk', sans-serif" }}
            >
              Update API Key
            </h3>
            <p className="mt-1 text-xs text-[#9CA3AF]">
              Replace the API key for{" "}
              <span className="font-medium text-[#374151]">
                {editingProvider.display_name}
              </span>{" "}
              ({editingProvider.provider_type}). The new key will be validated
              and models re-synced.
            </p>
            <div className="mt-4 flex flex-col gap-1.5">
              <span className={labelClass}>New API Key</span>
              <PasswordInput
                className={inputClass}
                value={editAPIKey}
                onChange={(e) => setEditAPIKey(e.target.value)}
                placeholder={
                  providerDefaults[editingProvider.provider_type]
                    ?.placeholder ?? "API key"
                }
              />
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <button
                type="button"
                className={btnOutline}
                onClick={() => setEditingProvider(null)}
              >
                Cancel
              </button>
              <button
                type="button"
                className={btnPrimary}
                disabled={!editAPIKey.trim() || providerStatus !== "idle"}
                onClick={() => void onConfirmEditProvider()}
              >
                {providerStatus === "saving" ? "Updating..." : "Update Key"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Provider Modal */}
      {deletingProvider && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
          onClick={() => setDeletingProvider(null)}
        >
          <div
            className="w-full max-w-sm rounded-xl bg-white p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h3
              className="text-base font-semibold text-[#111827]"
              style={{ fontFamily: "'Space Grotesk', sans-serif" }}
            >
              Delete Provider
            </h3>
            <p className="mt-2 text-sm text-[#6B7280]">
              Are you sure you want to delete{" "}
              <span className="font-medium text-[#111827]">
                {deletingProvider.display_name}
              </span>
              ? This will remove the credential and all associated models. This
              action cannot be undone.
            </p>
            <div className="mt-5 flex justify-end gap-2">
              <button
                type="button"
                className={btnOutline}
                onClick={() => setDeletingProvider(null)}
              >
                Cancel
              </button>
              <button
                type="button"
                className="rounded-lg bg-red-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-60"
                disabled={providerStatus !== "idle"}
                onClick={() => void onConfirmDeleteProvider()}
              >
                {providerStatus === "saving" ? "Deleting..." : "Delete"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>

      {costLimitKey && (
        <CostLimitModal
          userKey={costLimitKey}
          onClose={() => setCostLimitKey(null)}
          onUpdated={() => {
            setCostLimitKey(null);
            void loadManagementData(selectedProviderID ?? undefined);
          }}
        />
      )}
    </>
  );
}
