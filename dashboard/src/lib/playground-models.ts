import type { SelectOption } from '#/components/ui/Select'
import type { DashboardPlaygroundModel } from './api'

const preferredPlaygroundModelID = 'gpt-4o-mini'

export function buildPlaygroundModelOptions(
  models: DashboardPlaygroundModel[],
): SelectOption[] {
  const deduped = new Map<string, SelectOption>()

  for (const model of models) {
    const modelID = model.provider_model_id.trim()
    if (!modelID) {
      continue
    }

    const providerDisplayName = model.provider_display_name.trim()
    const providerTypeLabel = formatProviderTypeLabel(model.provider_type)
    const descriptionParts = [providerDisplayName, providerTypeLabel].filter(
      (part, index, all) => part && all.indexOf(part) === index,
    )

    deduped.set(modelID, {
      value: modelID,
      label: model.display_name.trim() || modelID,
      description: descriptionParts.length > 0 ? descriptionParts.join(' | ') : undefined,
    })
  }

  return Array.from(deduped.values()).sort((left, right) =>
    left.label.localeCompare(right.label),
  )
}

export function getNextPlaygroundModelSelection(
  currentValue: string,
  options: SelectOption[],
): string {
  if (options.some((option) => option.value === currentValue)) {
    return currentValue
  }
  const preferredOption = options.find(
    (option) => option.value === preferredPlaygroundModelID,
  )
  if (preferredOption) {
    return preferredOption.value
  }
  return options[0]?.value ?? ''
}

export function handlePlaygroundPromptKeyDown(
  event: {
    key: string
    shiftKey: boolean
    preventDefault: () => void
  },
  canSubmit: boolean,
  onSubmit: () => void,
) {
  if (event.key !== 'Enter' || event.shiftKey || !canSubmit) {
    return
  }

  event.preventDefault()
  onSubmit()
}

export function buildProviderOptions(
  models: DashboardPlaygroundModel[],
): SelectOption[] {
  const seen = new Set<string>()
  const options: SelectOption[] = []

  for (const model of models) {
    const pt = model.provider_type.trim().toLowerCase()
    if (!pt || seen.has(pt)) continue
    seen.add(pt)
    options.push({
      value: pt,
      label: formatProviderTypeLabel(model.provider_type),
    })
  }

  return options.sort((a, b) => a.label.localeCompare(b.label))
}

export function filterModelsByProvider(
  models: DashboardPlaygroundModel[],
  providerType: string,
): DashboardPlaygroundModel[] {
  if (!providerType) return models
  return models.filter(
    (m) => m.provider_type.trim().toLowerCase() === providerType,
  )
}

function formatProviderTypeLabel(providerType: string): string {
  switch (providerType.trim().toLowerCase()) {
    case 'openai':
      return 'OpenAI'
    case 'anthropic':
      return 'Anthropic'
    case 'google':
      return 'Google'
    default: {
      const normalized = providerType.trim()
      if (!normalized) {
        return ''
      }
      return normalized.charAt(0).toUpperCase() + normalized.slice(1)
    }
  }
}
