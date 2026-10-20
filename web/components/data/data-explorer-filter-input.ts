/** Comma-separated set drafts use CSV quotes, so literal commas remain editable. */
function filterInputTokens(value: string): string[] {
  const tokens: string[] = []
  let start = 0
  let quoted = false
  for (let index = 0; index < value.length; index += 1) {
    if (value[index] === '"') {
      if (quoted && value[index + 1] === '"') index += 1
      else if (quoted || !value.slice(start, index).trim()) quoted = !quoted
    } else if (value[index] === ',' && !quoted) {
      tokens.push(value.slice(start, index))
      start = index + 1
    }
  }
  tokens.push(value.slice(start))
  return tokens
}

function decodeFilterInputToken(token: string): string {
  const value = token.trim()
  return /^"(?:[^"]|"")*"$/.test(value) ? value.slice(1, -1).replaceAll('""', '"') : value
}

function encodeFilterInputToken(value: string): string {
  return /[",]/.test(value) || value !== value.trim() ? `"${value.replaceAll('"', '""')}"` : value
}

export function isSetFilterOperator(operator: string): boolean {
  return operator === 'in' || operator === 'not_in'
}

export function filterInputValues(value: string, operator: string): string[] {
  return (isSetFilterOperator(operator) ? filterInputTokens(value).map(decodeFilterInputToken) : [value.trim()]).filter(Boolean)
}

export function formatFilterInputValues(values: string[], operator: string): string {
  return (isSetFilterOperator(operator) ? values.map(encodeFilterInputToken) : values).join(', ')
}

export function selectFilterInputValue(draft: string, operator: string, value: string): string {
  if (!isSetFilterOperator(operator)) return value
  const tokens = filterInputTokens(draft)
  tokens[tokens.length - 1] = `${tokens.length > 1 ? ' ' : ''}${encodeFilterInputToken(value)}`
  return tokens.join(',')
}

export function filterInputSearch(value: string, operator: string): string {
  return isSetFilterOperator(operator) ? decodeFilterInputToken(filterInputTokens(value).at(-1) ?? '') : value
}
