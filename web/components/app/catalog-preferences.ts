// Legacy browser-wide keys have no known owner and must not be attributed
// to whichever account happens to sign in next.
export function scopedCatalogFavoritesStorageKey(principalID: string): string {
  return `leapview.dashboard-catalog.favorites.v1:${principalID}`
}

export function scopedCatalogRecentsStorageKey(principalID: string): string {
  return `leapview.dashboard-catalog.recents.v1:${principalID}`
}

export function readStringList(key: string): string[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(key) ?? '[]')
    return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === 'string' && Boolean(entry.trim())) : []
  } catch {
    return []
  }
}

export function readStringRecord(key: string): Record<string, string> {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(key) ?? '{}')
    if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
    return Object.fromEntries(Object.entries(value).filter((entry): entry is [string, string] => typeof entry[1] === 'string'))
  } catch {
    return {}
  }
}

export function writeStorage(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Discovery preferences are a progressive enhancement when storage is unavailable.
  }
}
