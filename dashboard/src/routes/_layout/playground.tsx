import { createFileRoute, Link } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import { Send, FileText } from "lucide-react";
import {
  APIError,
  fetchAdminUserKeys,
  fetchDashboardPlaygroundModels,
  submitPlaygroundRequest,
  type AdminUserKey,
  type PlaygroundResponse,
} from "#/lib/api";
import {
  buildPlaygroundModelOptions,
  buildProviderOptions,
  filterModelsByProvider,
  getNextPlaygroundModelSelection,
  handlePlaygroundPromptKeyDown,
} from "#/lib/playground-models";
import { buildPlaygroundUserOptions } from "#/lib/playground-users";
import { TopBar } from "#/components/ui/TopBar";
import { StatusBadge } from "#/components/ui/StatusBadge";
import { Select } from "#/components/ui/Select";

export const Route = createFileRoute("/_layout/playground")({
  component: PlaygroundPage,
});

export function PlaygroundPage() {
  const [userId, setUserId] = useState("");
  const [userKeys, setUserKeys] = useState<AdminUserKey[]>([]);
  const [playgroundModels, setPlaygroundModels] = useState<
    Awaited<ReturnType<typeof fetchDashboardPlaygroundModels>>["models"]
  >([]);
  const [provider, setProvider] = useState("");
  const [model, setModel] = useState("");
  const [prompt, setPrompt] = useState("");
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState(1024);
  const [submitting, setSubmitting] = useState(false);
  const [loadingUsers, setLoadingUsers] = useState(true);
  const [loadingModels, setLoadingModels] = useState(true);
  const [userLoadError, setUserLoadError] = useState<string | null>(null);
  const [modelLoadError, setModelLoadError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<{
    httpStatus: number;
    payload: PlaygroundResponse | { error: string };
  } | null>(null);
  const userOptions = useMemo(
    () => buildPlaygroundUserOptions(userKeys),
    [userKeys],
  );
  const providerOptions = useMemo(
    () => buildProviderOptions(playgroundModels),
    [playgroundModels],
  );
  const filteredModels = useMemo(
    () => filterModelsByProvider(playgroundModels, provider),
    [playgroundModels, provider],
  );
  const modelOptions = useMemo(
    () => buildPlaygroundModelOptions(filteredModels),
    [filteredModels],
  );

  useEffect(() => {
    let cancelled = false;

    async function loadAvailableUsers() {
      setLoadingUsers(true);
      setUserLoadError(null);
      try {
        const payload = await fetchAdminUserKeys();
        if (cancelled) {
          return;
        }

        setUserKeys(payload.user_keys);
        const options = buildPlaygroundUserOptions(payload.user_keys);
        setUserId((current) => {
          if (options.some((option) => option.value === current)) {
            return current;
          }
          return options[0]?.value ?? "";
        });
      } catch (err) {
        if (cancelled) {
          return;
        }

        setUserKeys([]);
        setUserId("");
        if (err instanceof APIError) {
          setUserLoadError(err.message);
        } else {
          setUserLoadError(
            err instanceof Error
              ? err.message
              : "Failed to load available users.",
          );
        }
      } finally {
        if (!cancelled) {
          setLoadingUsers(false);
        }
      }
    }

    void loadAvailableUsers();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadAvailableModels() {
      setLoadingModels(true);
      setModelLoadError(null);
      try {
        const payload = await fetchDashboardPlaygroundModels();
        if (cancelled) {
          return;
        }

        setPlaygroundModels(payload.models);
      } catch (err) {
        if (cancelled) {
          return;
        }

        setPlaygroundModels([]);
        setModel("");
        if (err instanceof APIError) {
          setModelLoadError(err.message);
        } else {
          setModelLoadError(
            err instanceof Error
              ? err.message
              : "Failed to load available models.",
          );
        }
      } finally {
        if (!cancelled) {
          setLoadingModels(false);
        }
      }
    }

    void loadAvailableModels();

    return () => {
      cancelled = true;
    };
  }, []);

  // When provider changes, clear model and let user pick from filtered list
  useEffect(() => {
    setModel("");
  }, [provider]);

  const canSubmit = Boolean(
    !submitting &&
    !loadingUsers &&
    !loadingModels &&
    userId.trim() &&
    model.trim() &&
    prompt.trim(),
  );

  async function submitRequest() {
    if (!userId.trim()) {
      setError("Select a user to continue.");
      return;
    }
    if (!model.trim()) {
      setError("Select a model to continue.");
      return;
    }
    if (!prompt.trim()) {
      setError("Enter a prompt to continue.");
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      const response = await submitPlaygroundRequest({
        userId,
        model,
        prompt,
      });
      setResult(response);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Playground request failed",
      );
    } finally {
      setSubmitting(false);
    }
  }

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void submitRequest();
  }

  const proxyPayload =
    result && "proxy_status" in result.payload
      ? (result.payload as PlaygroundResponse)
      : null;

  const inputClass =
    "h-10 w-full rounded-lg border border-[#D1D5DB] bg-[#F9FAFB] px-3 text-sm text-[#374151] outline-none focus:border-[#00C896] focus:ring-1 focus:ring-[#00C896]";
  const labelClass = "text-xs font-medium text-[#6B7280]";

  return (
    <div className="flex h-full flex-col" style={{ background: "#F8F9FA" }}>
      <TopBar title="Playground" />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden lg:flex-row">
        {/* Left — Config Panel */}
        <div className="flex w-full shrink-0 flex-col border-b border-[#E5E7EB] bg-white lg:h-full lg:w-[420px] lg:border-b-0 lg:border-r">
          <form
            id="playground-form"
            className="flex h-full min-h-0 flex-col"
            onSubmit={onSubmit}
          >
            <div className="flex-1 overflow-y-auto p-4 sm:p-6">
              <div className="flex flex-col gap-4 pb-4 sm:gap-5">
                <div className="flex flex-col gap-1.5">
                  <span className={labelClass}>User</span>
                  <Select
                    value={userId}
                    onChange={setUserId}
                    options={userOptions}
                    placeholder={
                      loadingUsers ? "Loading users..." : "Select a user"
                    }
                    disabled={loadingUsers || userOptions.length === 0}
                    searchable
                    searchPlaceholder="Search users by name or ID"
                    emptyMessage="No matching users found."
                    aria-label="Select user for playground"
                  />
                  {userLoadError ? (
                    <span className="text-xs text-red-600">
                      {userLoadError}
                    </span>
                  ) : userOptions.length === 0 && !loadingUsers ? (
                    <span className="text-xs text-[#9CA3AF]">
                      No active users are available yet. Create one in{" "}
                      <Link
                        to="/api-management"
                        className="text-[#00C896] hover:underline"
                      >
                        API Management
                      </Link>
                      .
                    </span>
                  ) : (
                    <span className="text-xs text-[#9CA3AF]">
                      Search by display name or user ID. The playground resolves
                      the active API key automatically.
                    </span>
                  )}
                </div>

                <div className="flex flex-col gap-1.5">
                  <span className={labelClass}>Provider</span>
                  <Select
                    value={provider}
                    onChange={setProvider}
                    options={providerOptions}
                    placeholder={
                      loadingModels
                        ? "Loading providers..."
                        : "Select a provider"
                    }
                    disabled={loadingModels || providerOptions.length === 0}
                    aria-label="Select provider for playground"
                  />
                  <span className="text-xs text-[#9CA3AF]">
                    Select a provider to see its available models.
                  </span>
                </div>

                <div className="flex flex-col gap-1.5">
                  <span className={labelClass}>Model</span>
                  <Select
                    value={model}
                    onChange={setModel}
                    options={modelOptions}
                    placeholder={
                      !provider
                        ? "Select a provider first"
                        : loadingModels
                          ? "Loading models..."
                          : "Select a model"
                    }
                    disabled={
                      !provider || loadingModels || modelOptions.length === 0
                    }
                    searchable
                    searchPlaceholder="Search models by name or ID"
                    emptyMessage="No matching models found."
                    aria-label="Select model for playground"
                  />
                  {modelLoadError ? (
                    <span className="text-xs text-red-600">
                      {modelLoadError}
                    </span>
                  ) : modelOptions.length === 0 && !loadingModels ? (
                    <span className="text-xs text-[#9CA3AF]">
                      No enabled models are available yet. Turn one on in{" "}
                      <Link
                        to="/api-management"
                        className="text-[#00C896] hover:underline"
                      >
                        API Management
                      </Link>
                      .
                    </span>
                  ) : (
                    <span className="text-xs text-[#9CA3AF]">
                      Only enabled synced models that the Playground can execute
                      appear here.
                    </span>
                  )}
                </div>

                <label className="flex flex-col gap-1.5">
                  <div className="flex items-center justify-between">
                    <span className={labelClass}>Temperature</span>
                    <span className="text-xs font-semibold text-[#111827]">
                      {temperature.toFixed(1)}
                    </span>
                  </div>
                  <input
                    type="range"
                    min="0"
                    max="2"
                    step="0.1"
                    value={temperature}
                    onChange={(e) => setTemperature(parseFloat(e.target.value))}
                    className="h-2 w-full cursor-pointer appearance-none rounded-full bg-[#E5E7EB] accent-[#00C896]"
                  />
                  <div className="flex justify-between text-[10px] text-[#9CA3AF]">
                    <span>Precise</span>
                    <span>Creative</span>
                  </div>
                </label>

                <label className="flex flex-col gap-1.5">
                  <span className={labelClass}>Max Tokens</span>
                  <input
                    className={inputClass}
                    type="number"
                    min="1"
                    max="4096"
                    value={maxTokens}
                    onChange={(e) =>
                      setMaxTokens(parseInt(e.target.value) || 1024)
                    }
                  />
                </label>

                <label className="flex flex-col gap-1.5">
                  <div className="flex items-center justify-between">
                    <span className={labelClass}>Prompt</span>
                    <span className="text-[10px] text-[#9CA3AF]">
                      {prompt.length} chars
                    </span>
                  </div>
                  <textarea
                    className="min-h-[96px] sm:min-h-[140px] w-full rounded-lg border border-[#D1D5DB] bg-[#F9FAFB] p-3 text-sm text-[#374151] outline-none focus:border-[#00C896] focus:ring-1 focus:ring-[#00C896] resize-none"
                    value={prompt}
                    onChange={(e) => setPrompt(e.target.value)}
                    onKeyDown={(event) =>
                      handlePlaygroundPromptKeyDown(event, canSubmit, () => {
                        void submitRequest();
                      })
                    }
                    placeholder="Enter your prompt here..."
                  />
                  <span className="text-xs text-[#9CA3AF]">
                    Press Enter to send. Use Shift+Enter for a new line.
                  </span>
                </label>
              </div>
            </div>

            <div className="border-t border-[#E5E7EB] bg-white p-4 sm:p-6">
              <div className="flex flex-col gap-3">
                {error && <p className="text-sm text-red-600">{error}</p>}

                <button
                  type="submit"
                  disabled={
                    submitting ||
                    loadingUsers ||
                    loadingModels ||
                    !userId.trim() ||
                    !model.trim()
                  }
                  className="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-[#00C896] text-sm font-medium text-white hover:bg-[#00B085] disabled:opacity-60"
                >
                  <Send size={14} />
                  {submitting ? "Sending..." : "Send Request"}
                </button>
              </div>
            </div>
          </form>
        </div>

        {/* Right — Response Viewer */}
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          <div className="flex-1 overflow-auto p-6">
            {!result ? (
              <div className="flex h-full flex-col items-center justify-center text-center">
                <div className="mb-3 flex h-14 w-14 items-center justify-center rounded-full bg-[#F3F4F6]">
                  <Send size={24} className="text-[#9CA3AF]" />
                </div>
                <p className="text-sm font-medium text-[#6B7280]">
                  No response yet
                </p>
                <p className="text-xs text-[#9CA3AF]">
                  Send a request to see the API response here
                </p>
              </div>
            ) : (
              <div className="space-y-4">
                {/* Response header */}
                <div className="flex items-center gap-3">
                  <StatusBadge status={result.httpStatus} />
                  {proxyPayload && (
                    <span className="text-xs text-[#6B7280]">
                      {proxyPayload.latency_ms}ms
                    </span>
                  )}
                </div>

                {/* Response body */}
                <div className="overflow-hidden rounded-xl border border-[#1E293B]">
                  <div className="bg-[#0A0F1C] px-4 py-2 text-xs font-medium text-[#94A3B8]">
                    Response
                  </div>
                  <pre className="max-h-[500px] overflow-auto bg-[#0F172A] p-4 text-xs leading-relaxed text-[#E2E8F0]">
                    {proxyPayload
                      ? JSON.stringify(proxyPayload.raw_json, null, 2)
                      : JSON.stringify(result.payload, null, 2)}
                  </pre>
                </div>

                {/* Token counters */}
                {proxyPayload?.usage && (
                  <div className="grid grid-cols-4 gap-3">
                    <div className="rounded-lg border border-[#E5E7EB] bg-white p-3 text-center">
                      <div className="text-[10px] uppercase tracking-wider text-[#9CA3AF]">
                        Prompt
                      </div>
                      <div className="text-lg font-bold text-[#111827]">
                        {proxyPayload.usage.prompt_tokens ?? "—"}
                      </div>
                    </div>
                    <div className="rounded-lg border border-[#E5E7EB] bg-white p-3 text-center">
                      <div className="text-[10px] uppercase tracking-wider text-[#9CA3AF]">
                        Completion
                      </div>
                      <div className="text-lg font-bold text-[#111827]">
                        {proxyPayload.usage.completion_tokens ?? "—"}
                      </div>
                    </div>
                    <div className="rounded-lg border border-[#E5E7EB] bg-white p-3 text-center">
                      <div className="text-[10px] uppercase tracking-wider text-[#9CA3AF]">
                        Total
                      </div>
                      <div className="text-lg font-bold text-[#111827]">
                        {proxyPayload.usage.total_tokens ?? "—"}
                      </div>
                    </div>
                    <div className="rounded-lg border border-[#E5E7EB] bg-white p-3 text-center">
                      <div className="text-[10px] uppercase tracking-wider text-[#9CA3AF]">
                        Response Time
                      </div>
                      <div className="text-lg font-bold text-[#111827]">
                        {proxyPayload.latency_ms}ms
                      </div>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
