/**
 * Memoizes an async loader (typically a dynamic `import()`) so concurrent and
 * later callers share one load, while a failed load is forgotten.
 *
 * A bare `promise ??= import(...)` caches a rejected promise forever: one
 * transient chunk or network failure would then fail every later caller until
 * the page is reloaded. Here a rejection clears the cache, so the next call
 * starts a fresh attempt, and the original error still reaches every caller
 * that was waiting on the failed attempt.
 */
export function retryableLazy<T>(load: () => Promise<T>): () => Promise<T> {
  let pending: Promise<T> | undefined
  return () => {
    if (pending) return pending
    const attempt: Promise<T> = Promise.resolve().then(load).catch((error: unknown) => {
      if (pending === attempt) pending = undefined
      throw error
    })
    pending = attempt
    return attempt
  }
}
