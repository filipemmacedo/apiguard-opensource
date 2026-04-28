import type { DashboardTimeRange } from './api'

function isDateOnlyValue(value: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(value)
}

function parseDashboardRangeValue(value: string, boundary: 'start' | 'end'): Date {
  if (isDateOnlyValue(value)) {
    const time = boundary === 'start' ? 'T00:00:00.000' : 'T23:59:59.999'
    return new Date(`${value}${time}`)
  }

  return new Date(value)
}

export function buildDashboardTimeRange(
  fromValue: string,
  toValue: string,
): { range?: DashboardTimeRange; error?: string } {
  const fromTrimmed = fromValue.trim()
  const toTrimmed = toValue.trim()

  if (!fromTrimmed && !toTrimmed) {
    return { range: undefined }
  }
  if (!fromTrimmed || !toTrimmed) {
    return { error: 'Both From and To must be provided.' }
  }

  const fromDate = parseDashboardRangeValue(fromTrimmed, 'start')
  const toDate = parseDashboardRangeValue(toTrimmed, 'end')
  if (Number.isNaN(fromDate.valueOf()) || Number.isNaN(toDate.valueOf())) {
    return { error: 'From and To must be valid date values.' }
  }

  const allowSameDayRange = isDateOnlyValue(fromTrimmed) && isDateOnlyValue(toTrimmed)
  if (fromDate > toDate || (!allowSameDayRange && fromDate.getTime() === toDate.getTime())) {
    return { error: 'From must be earlier than To.' }
  }

  return {
    range: {
      from: fromDate.toISOString(),
      to: toDate.toISOString(),
    },
  }
}
