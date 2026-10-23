import type { ReactiveElement } from 'lit'
import { hideDatastarRuntimeError, showDatastarRuntimeError } from './datastar-runtime-error'
import { loadDatastarRuntime, type DatastarRuntime } from './datastar-runtime'

type Constructor<T = object> = new (...args: any[]) => T
type Dispose = () => void
type Runtime = Pick<DatastarRuntime, 'effect' | 'getPath' | 'root'>
export type SignalRoot = Record<string, unknown>

export interface DatastarLitHost {
  readonly signals: SignalRoot
  signal<T>(path: string, fallback: T): T
}

// A failed runtime import is retried a few times before the page is declared
// degraded; one transient chunk or network failure should not need a reload.
const defaultRetryDelaysMs: readonly number[] = [400, 1500]

type RuntimeLoaderOverrides = {
  load?: () => Promise<Runtime>
  retryDelaysMs?: readonly number[]
  reportFailure?: () => void
  reportRecovery?: () => void
}

let runtimeForTests: Runtime | null = null
let loaderOverrides: RuntimeLoaderOverrides = {}
// De-duplicates the retry cycle across components. It is cleared on failure,
// so the next connected component starts a fresh cycle instead of inheriting
// a rejected promise; the module itself is cached by loadDatastarRuntime.
let runtimeCycle: Promise<Runtime> | null = null
let loadedRuntime: Runtime | null = null

export function DatastarLit<T extends Constructor<ReactiveElement>>(
  Base: T,
): T & Constructor<DatastarLitHost> {
  abstract class DatastarLit extends Base {
    #renderDispose: Dispose | null = null
    #connected = false
    #signalSnapshots: WeakMap<object, unknown> | null = null

    override connectedCallback(): void {
      this.#connected = true
      super.connectedCallback()
      loadRuntime().then(async () => {
        if (!this.#connected) return
        this.requestUpdate()
        await this.updateComplete
        await afterInitialSignalScan()
        if (this.#connected) this.requestUpdate()
      }, () => {
        // The page-level notice was already raised by the retry cycle; this
        // component keeps its non-reactive first render until a reload.
      })
    }

    override performUpdate(): void {
      if (!this.isUpdatePending) return
      // Several route getters read the same transcript or datasets during
      // render and updated. Materialize each tree once for this update only;
      // the next effect must traverse it again to track fresh signal values.
      this.#signalSnapshots = new WeakMap()
      try {
        this.#performSignalUpdate()
      } finally {
        this.#signalSnapshots = null
      }
    }

    #performSignalUpdate(): void {
      const activeRuntime = runtime()
      if (!activeRuntime) {
        super.performUpdate()
        return
      }
      this.#renderDispose?.()
      this.#renderDispose = null

      let updateFromLit = true
      this.#renderDispose = activeRuntime.effect(() => {
        trackSignalRoot(activeRuntime.root)
        if (updateFromLit) {
          updateFromLit = false
          super.performUpdate()
          return
        }
        this.requestUpdate()
      })
    }

    override disconnectedCallback(): void {
      this.#connected = false
      this.#renderDispose?.()
      this.#renderDispose = null
      super.disconnectedCallback()
    }

    get signals(): SignalRoot {
      return runtime()?.root ?? {}
    }

    signal<T>(path: string, fallback: T): T {
      const value = runtime()?.getPath<T>(path)
      return materializeSignal(value === undefined ? fallback : value, this.#signalSnapshots)
    }
  }

  return DatastarLit as unknown as T & Constructor<DatastarLitHost>
}

export function setDatastarLitRuntimeForTests(runtime: Runtime | null): void {
  runtimeForTests = runtime
  loadedRuntime = runtime
  runtimeCycle = null
  loaderOverrides = {}
}

export function setDatastarLitLoaderForTests(overrides: RuntimeLoaderOverrides | null): void {
  runtimeForTests = null
  loadedRuntime = null
  runtimeCycle = null
  loaderOverrides = overrides ?? {}
}

function runtime(): Runtime | null {
  return runtimeForTests ?? loadedRuntime
}

function trackSignalRoot(root: SignalRoot): void {
  Object.keys(root)
}

function loadRuntime(): Promise<Runtime> {
  if (runtimeForTests) return Promise.resolve(runtimeForTests)
  if (loadedRuntime) return Promise.resolve(loadedRuntime)
  if (runtimeCycle) return runtimeCycle
  const cycle: Promise<Runtime> =loadRuntimeWithRetries().then((loaded) => {
    loadedRuntime = loaded
    ;(loaderOverrides.reportRecovery ?? hideDatastarRuntimeError)()
    return loaded
  }, (error: unknown) => {
    if (runtimeCycle === cycle) runtimeCycle = null
    console.error('LeapView could not load the Datastar runtime', error)
    ;(loaderOverrides.reportFailure ?? showDatastarRuntimeError)()
    throw error
  })
  runtimeCycle = cycle
  return cycle
}

async function loadRuntimeWithRetries(): Promise<Runtime> {
  const load = loaderOverrides.load ?? (() => loadDatastarRuntime() as Promise<Runtime>)
  const delays = loaderOverrides.retryDelaysMs ?? defaultRetryDelaysMs
  for (let attempt = 0; ; attempt++) {
    try {
      return await load()
    } catch (error) {
      if (attempt >= delays.length) throw error
      await new Promise<void>((resolve) => setTimeout(resolve, delays[attempt]))
    }
  }
}

async function afterInitialSignalScan(): Promise<void> {
  await Promise.resolve()
  await new Promise<void>((resolve) => {
    if (typeof requestAnimationFrame === 'function') {
      requestAnimationFrame(() => resolve())
      return
    }
    setTimeout(resolve, 0)
  })
}

function materializeSignal<T>(value: T, snapshots: WeakMap<object, unknown> | null): T {
  if (value && typeof value === 'object' && snapshots?.has(value)) {
    return snapshots.get(value) as T
  }
  if (Array.isArray(value)) {
    const out: unknown[] = new Array(value.length)
    snapshots?.set(value, out)
    value.forEach((item, index) => { out[index] = materializeSignal(item, snapshots) })
    return out as T
  }
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {}
    snapshots?.set(value, out)
    for (const key of Object.keys(value)) {
      out[key] = materializeSignal((value as Record<string, unknown>)[key], snapshots)
    }
    return out as T
  }
  return value
}
