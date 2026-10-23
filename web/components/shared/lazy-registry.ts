type ElementLoader = () => Promise<unknown>

const loading = new Map<string, Promise<void>>()
