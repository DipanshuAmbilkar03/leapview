import { isNumericType } from './field-types'
import { Binary, Braces, CalendarDays, CircleHelp, Clock3, Hash, ToggleLeft, Type, type IconNode } from 'lucide'

export function fieldTypeIcon(type = ''): IconNode {
  const normalized = type.toLowerCase()
  if (normalized.includes('time')) return Clock3
  if (normalized.includes('date')) return CalendarDays
  if (normalized.includes('bool')) return ToggleLeft
  if (isNumericType(normalized)) return Hash
  if (/blob|binary|byte/.test(normalized)) return Binary
  if (/json|struct|map|list|array/.test(normalized)) return Braces
  if (/char|text|string|uuid|enum/.test(normalized)) return Type
  return CircleHelp
}
