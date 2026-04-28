import type { PlaygroundResponse } from '../lib/api'

type PlaygroundResultProps = {
  httpStatus: number
  payload: PlaygroundResponse | { error: string }
}

function hasProxyPayload(
  payload: PlaygroundResponse | { error: string },
): payload is PlaygroundResponse {
  return 'proxy_status' in payload
}

export default function PlaygroundResult({
  httpStatus,
  payload,
}: PlaygroundResultProps) {
  if (!hasProxyPayload(payload)) {
    return (
      <section className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-800">
        <p className="m-0 font-semibold">Request failed</p>
        <pre className="mt-2 overflow-auto whitespace-pre-wrap">
          {JSON.stringify(payload, null, 2)}
        </pre>
      </section>
    )
  }

  const usage = payload.usage
  return (
    <section className="rounded-lg border border-[#1E293B] bg-[#0F172A] p-4 text-sm text-[#E2E8F0]">
      <p className="m-0">
        <strong>HTTP status:</strong> {httpStatus}
      </p>
      <p className="m-0">
        <strong>Proxy status:</strong> {payload.proxy_status}
      </p>
      <p className="m-0">
        <strong>Latency (ms):</strong> {payload.latency_ms}
      </p>
      {usage ? (
        <div className="mt-2">
          <p className="m-0 font-semibold">Usage</p>
          <p className="m-0">prompt_tokens: {usage.prompt_tokens ?? 'n/a'}</p>
          <p className="m-0">
            completion_tokens: {usage.completion_tokens ?? 'n/a'}
          </p>
          <p className="m-0">total_tokens: {usage.total_tokens ?? 'n/a'}</p>
        </div>
      ) : (
        <p className="m-0 mt-2">Usage unavailable</p>
      )}
      <p className="mb-1 mt-3 font-semibold">Raw JSON</p>
      <pre className="max-h-80 overflow-auto rounded p-3 text-xs">
        {JSON.stringify(payload.raw_json, null, 2)}
      </pre>
    </section>
  )
}
