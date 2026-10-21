import { html, nothing, type TemplateResult } from 'lit'
import { ChevronDown, Download, Ellipsis, Link, Share2 } from 'lucide'
import type { DataExploreFieldSignal, DataExplorerCommand, SavedExplorationCommandSignal, SavedExplorationStateSignal } from '../../generated/signals'
import type { ExplorationSpec } from '../../generated/exploration'
import { lucideIcon } from '../shared/lucide-icons'
import { dataExplorerExportURL, dataExplorerURL, savedExplorationShareURL, updateDataExplorerURL, type DataExplorerHistoryMode } from './data-explorer-url'
import { browserCommandFailure, type BrowserCommandFailure } from '../shared/command-failure'
import { uuidv7 } from '../shared/command-identity'

export const emptySavedExplorations: SavedExplorationStateSignal = {
  enabled: false,
  list: { items: [], includeArchived: false },
  command: { action: 'create' },
  save: { state: 'saved' },
}

export { savedExplorationStyles } from './data-explorer-saved-styles'

export type SavedExplorationVisibility = 'private' | 'organization'
export type SavedExplorationCurrent = NonNullable<SavedExplorationStateSignal['current']>

export type SavedExplorationTrackerCallbacks = {
  onBaselineChanged(current?: SavedExplorationCurrent): void
  onDirty(): void
}

export class SavedExplorationTracker {
  private dirtyFingerprint = ''
  private dirtyBaseline = ''

  constructor(private readonly callbacks: SavedExplorationTrackerCallbacks) {}

  observe(state: SavedExplorationStateSignal, currentSpec?: ExplorationSpec): void {
    const current = state.current
    const currentBaseline = current
      ? `${current.id}\u0000${current.revision.revisionId}\u0000${current.visibility}\u0000${JSON.stringify(current.spec ?? null)}`
      : ''
    if (currentBaseline !== this.dirtyBaseline) {
      this.dirtyBaseline = currentBaseline
      this.dirtyFingerprint = ''
      this.callbacks.onBaselineChanged(current)
    }
    if (!current?.detached || current.status !== 'active' || !current.spec || !currentSpec) return
    const fingerprint = JSON.stringify(currentSpec)
    if (fingerprint !== JSON.stringify(current.spec) && fingerprint !== this.dirtyFingerprint) {
      this.dirtyFingerprint = fingerprint
      this.callbacks.onDirty()
    }
  }
}

export type SavedExplorationViewOptions = {
  actionPanel(): 'save' | 'more' | undefined
  onToggleActions(panel?: 'save' | 'more'): void
  savedTitle(): string
  suggestedTitle?(): string
  savedDuplicateTitle(): string
  savedVisibility(): SavedExplorationVisibility
  currentSavedVisibility(current: SavedExplorationCurrent): SavedExplorationVisibility
  canSaveCurrent(): boolean
  activeSpec(): ExplorationSpec
  onSavedTitleInput(value: string): void
  onDuplicateTitleInput(value: string): void
  onSavedVisibilityInput(value: SavedExplorationVisibility): void
  onCurrentSavedVisibilityInput(value: SavedExplorationVisibility): void
  onCommand(command: SavedExplorationCommandSignal): void
  onReopen(current: SavedExplorationCurrent): void
  shareStatus(): string
  shareFallbackURL(): string
  onShareStatus(message: string, fallbackURL: string): void
  dashboardAppend?(): TemplateResult | typeof nothing
  commandPending?(): boolean
  transportFailure?(): BrowserCommandFailure | null
  retryReady?(): boolean
  retryCommand?(): void
}

type PendingSavedExplorationCommand = {
  command: SavedExplorationCommandSignal
  request: SavedExplorationStateSignal
  identity: { requestId: string; idempotencyKey: string }
  active: boolean
  settled: boolean
  responseReceived: boolean
  failure: BrowserCommandFailure | null
}

export class SavedExplorationViewController {
  savedActionPanel: 'save' | 'more' | undefined
  savedTitle = ''
  savedDuplicateTitle = ''
  savedVisibility: SavedExplorationVisibility = 'private'
  currentSavedVisibility: SavedExplorationVisibility = 'private'
  savedShareStatus = ''
  savedShareFallbackURL = ''
  private pendingCommand: PendingSavedExplorationCommand | null = null

  constructor(
    private readonly host: EventTarget,
    private readonly refresh: () => void,
    private readonly getState: () => SavedExplorationStateSignal = () => emptySavedExplorations,
  ) {}

  private commandOwner(): HTMLElement | null {
    return (this.host as HTMLElement).querySelector<HTMLElement>('[data-saved-exploration-command-owner]')
  }

  // Saved mutations have a dedicated Datastar owner. Analytical commands can
  // overlap without confirming or failing this command's transport state.
  handleFetch(event: Event): boolean {
    const detail = (event as CustomEvent<{ type?: string; el?: Element; argsRaw?: { signals?: string } }>).detail
    const owner = this.commandOwner()
    if (!owner || detail?.el !== owner) return false
    const pending = this.pendingCommand
    if (!pending) return true
    if (detail.type === 'datastar-patch-signals') {
      try {
        const patch: unknown = JSON.parse(detail.argsRaw?.signals ?? '')
        if (patch && typeof patch === 'object' && Object.hasOwn(patch, 'savedExplorations')) pending.responseReceived = true
      } catch { /* An invalid response cannot confirm a saved mutation. */ }
      return true
    }
    const failure = browserCommandFailure(event, 'Saving the exploration')
    if (!failure && detail.type !== 'finished') return true
    // An error can arrive before the response body and fetch finally block
    // finish. Keep this owner reserved until finished so a late terminal event
    // cannot be mistaken for the next explicit retry's completion.
    if (detail.type === 'finished') pending.settled = true
    if (pending.responseReceived) {
      if (!pending.settled) return true
      this.pendingCommand = null
      this.refresh()
      return true
    }
    if (!pending.active) {
      this.refresh()
      return true
    }
    pending.active = false
    pending.failure = failure ? {
      ...failure,
      message: failure.retryable
        ? `${failure.message} The outcome is unknown. Retry the same saved operation to confirm it.`
        : failure.message,
    } : {
      kind: 'unknown', status: null, retryable: true,
      message: 'Saving did not return a confirmed result. Your draft was kept. Retry the same saved operation to confirm its outcome.',
    }
    owner.dispatchEvent(new CustomEvent('lv-saved-exploration-transport-state', {
      detail: { state: 'error', message: pending.failure.message },
    }))
    this.refresh()
    return true
  }

  private submit(command: SavedExplorationCommandSignal): void {
    if (this.pendingCommand) return
    const request = JSON.parse(JSON.stringify(this.getState())) as SavedExplorationStateSignal
    request.command = JSON.parse(JSON.stringify(command)) as SavedExplorationCommandSignal
    request.save = { state: 'saving' }
    this.pendingCommand = {
      command: request.command, request,
      identity: { requestId: uuidv7(), idempotencyKey: uuidv7() },
      active: true, settled: false, responseReceived: false, failure: null,
    }
    this.dispatchPendingCommand()
  }

  private dispatchPendingCommand(): void {
    const pending = this.pendingCommand
    if (!pending) return
    // Clone before assigning reactive signals so subsequent signal patches
    // cannot mutate the retained request used for an explicit same-key retry.
    const detail = JSON.parse(JSON.stringify({ ...pending.command, request: pending.request, identity: pending.identity }))
    ;(this.commandOwner() ?? this.host).dispatchEvent(new CustomEvent('lv-saved-exploration-command', {
      bubbles: true, composed: true, detail,
    }))
    this.refresh()
  }

  private retryCommand = (): void => {
    const pending = this.pendingCommand
    if (!pending?.failure?.retryable || pending.active || !pending.settled) return
    pending.active = true
    pending.settled = false
    pending.failure = null
    pending.responseReceived = false
    this.dispatchPendingCommand()
  }

  readonly handleOutsidePointer = (event: PointerEvent): void => {
    const root = (this.host as HTMLElement).shadowRoot
    root?.querySelectorAll<HTMLDetailsElement>('.saved-exploration-sharing, .saved-exploration-picker').forEach((menu) => {
      if (menu.open && !event.composedPath().includes(menu)) menu.open = false
    })
    const saved = root?.querySelector('.saved-explorations')
    if (this.savedActionPanel && saved && !event.composedPath().includes(saved)) {
      this.savedActionPanel = undefined
      this.refresh()
    }
  }

  baselineChanged(current: SavedExplorationCurrent | null | undefined): void {
    this.savedActionPanel = undefined
    this.savedDuplicateTitle = ''
    this.currentSavedVisibility = current?.visibility ?? 'private'
    this.refresh()
  }

  options(
    canSaveCurrent: boolean,
    activeSpec: ExplorationSpec,
    dashboardAppend?: () => TemplateResult | typeof nothing,
    fields: DataExploreFieldSignal[] = [],
  ): SavedExplorationViewOptions {
    const update = <T>(setter: (value: T) => void) => (value: T) => { setter(value); this.refresh() }
    return {
      actionPanel: () => this.savedActionPanel,
      onToggleActions: (panel) => {
        const picker = (this.host as HTMLElement).shadowRoot?.querySelector<HTMLDetailsElement>('.saved-exploration-picker')
        if (panel && picker) picker.open = false
        this.savedActionPanel = this.savedActionPanel === panel ? undefined : panel
        this.refresh()
      },
      savedTitle: () => this.savedTitle,
      suggestedTitle: () => suggestedExplorationTitle(activeSpec, fields),
      savedDuplicateTitle: () => this.savedDuplicateTitle,
      savedVisibility: () => this.savedVisibility,
      currentSavedVisibility: (current) => this.currentSavedVisibility || current.visibility,
      canSaveCurrent: () => canSaveCurrent,
      activeSpec: () => activeSpec,
      onSavedTitleInput: update((value: string) => { this.savedTitle = value }),
      onDuplicateTitleInput: update((value: string) => { this.savedDuplicateTitle = value }),
      onSavedVisibilityInput: update((value: SavedExplorationVisibility) => { this.savedVisibility = value }),
      onCurrentSavedVisibilityInput: update((value: SavedExplorationVisibility) => { this.currentSavedVisibility = value }),
      onCommand: (command) => this.submit(command),
      onReopen: (current) => this.host.dispatchEvent(new CustomEvent('lv-saved-exploration-reopen', {
        bubbles: true, composed: true, detail: { explorationId: current.id, includeArchived: current.status === 'archived' },
      })),
      shareStatus: () => this.savedShareStatus,
      shareFallbackURL: () => this.savedShareFallbackURL,
      onShareStatus: (message, fallbackURL) => update((value: [string, string]) => {
        this.savedShareStatus = value[0]
        this.savedShareFallbackURL = value[1]
      })([message, fallbackURL]),
      dashboardAppend,
      commandPending: () => Boolean(this.pendingCommand),
      transportFailure: () => this.pendingCommand?.failure ?? null,
      retryReady: () => Boolean(this.pendingCommand?.settled && !this.pendingCommand.active),
      retryCommand: this.retryCommand,
    }
  }
}

export function synchronizeSavedExplorationURL(
  command: DataExplorerCommand,
  state: SavedExplorationStateSignal,
  embedded: boolean,
  signalAvailable: boolean,
): void {
  if (!embedded && signalAvailable) updateSavedExplorationURL(command, 'replace', state)
}

export function updateSavedExplorationURL(command: DataExplorerCommand, mode: DataExplorerHistoryMode, state: SavedExplorationStateSignal): void {
  updateDataExplorerURL(command, mode, state.list?.selectedId, savedExplorationSelectionIncludesArchived(state))
}

// Field labels carry authored meaning; the field contract does not expose aggregation.
export function suggestedExplorationTitle(spec: ExplorationSpec, fields: DataExploreFieldSignal[] = []): string {
  const fieldTitle = (id: string) => fields.find((field) => field.id === id)?.label?.trim()
    || id.split('.').at(-1)?.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()) || 'Field'
  const summarize = (names: string[]) => {
    const compact = names.slice(0, 2).map((name) => name.length > 32 ? `${name.slice(0, 31)}…` : name).join(', ')
    return names.length > 2 ? `${compact} +${names.length - 2}` : compact
  }
  const metrics = summarize(spec.metrics.map((field) => fieldTitle(field.field)))
  const dimensions = spec.dimensions.map((field) => fieldTitle(field.field))
  if (spec.time && !spec.dimensions.some((field) => field.field === spec.time?.field)) dimensions.push(fieldTitle(spec.time.field))
  const grouping = summarize(dimensions)
  return metrics ? grouping ? `${metrics} by ${grouping}` : metrics : grouping ? `Explore ${grouping}` : 'Untitled exploration'
}

export function explorationDisplayTitle(
  current: SavedExplorationCurrent | null | undefined,
  spec: ExplorationSpec,
  fields: DataExploreFieldSignal[] = [],
): string {
  if (current && current.title !== `Exploration · ${current.spec?.modelId || 'untitled'}`) return current.title
  return suggestedExplorationTitle(spec, fields)
}

export function renderSavedExplorations(state: SavedExplorationStateSignal, options: SavedExplorationViewOptions) {
  const current = state.current
  const items = state.list?.items ?? []
  const legacyItems = state.list?.legacyItems ?? []
  const savedCount = items.length + legacyItems.length
  const transportFailure = options.transportFailure?.()
  const unavailable = state.save?.state === 'error' || Boolean(transportFailure)
  if (!state.enabled) return nothing
  const hasCanonicalState = options.canSaveCurrent() && Boolean(options.activeSpec().modelId?.trim())
  const saving = state.save?.state === 'saving' || Boolean(options.commandPending?.())
  const unsaved = state.save?.state === 'dirty' || (current && options.currentSavedVisibility(current) !== current.visibility)
    || (current?.spec && hasCanonicalState && JSON.stringify(current.spec) !== JSON.stringify(options.activeSpec()))
  const saveStatus = unavailable ? transportFailure?.message || state.save.message || 'Could not save'
    : saving ? 'Saving…'
    : !current ? 'Not saved'
    : unsaved ? 'Unsaved changes'
    : current.status === 'archived' ? 'Archived · read only' : 'Saved'
  return html`
    <section class="saved-explorations" aria-label="Saved explorations" @keydown=${(event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      const section = event.currentTarget as HTMLElement
      const trigger = section.querySelector<HTMLButtonElement>('.saved-actions-toggle[aria-expanded="true"]')
      if (trigger) { options.onToggleActions(); trigger.focus() }
      const picker = section.querySelector<HTMLDetailsElement>('.saved-exploration-picker[open]')
      if (picker) { picker.open = false; picker.querySelector('summary')?.focus() }
      event.stopPropagation()
    }}>
      <div class="saved-explorations-header">
        <span class="saved-exploration-status" role=${unavailable ? 'alert' : 'status'}>${saveStatus}</span>
        ${transportFailure ? transportFailure.retryable
          ? html`<button type="button" class="text-button" ?disabled=${options.retryReady?.() === false} title=${options.retryReady?.() === false ? 'Waiting for the previous request to finish' : 'Confirm the original saved operation'} @click=${options.retryCommand}>Retry saved operation</button>`
          : html`<button type="button" class="text-button" @click=${() => window.location.reload()}>Reload latest state</button>`
          : nothing}
        ${savedCount ? html`
          <details class="saved-exploration-picker">
            <summary aria-label="Open saved explorations" title="Open saved explorations" @click=${() => options.onToggleActions()}><span>Saved explorations (${savedCount})</span>${lucideIcon(ChevronDown, { size: 13 })}</summary>
            <div class="saved-exploration-list">
              ${items.map((item) => html`<a class="saved-exploration-item" href=${savedExplorationShareURL(item.id, item.status === 'archived')} aria-current=${item.id === current?.id ? 'page' : nothing}>${item.title}</a>`)}
              ${legacyItems.map((item) => html`<a class="saved-exploration-item" href=${item.openHref}>${item.name}</a>`)}
            </div>
          </details>
        ` : nothing}
      </div>
      ${html`
        <div class="saved-exploration-toolbar">
          ${current?.canEdit && current.detached && current.status === 'active' ? html`<button type="button" class="text-button" ?disabled=${saving} @click=${() => saveSavedExploration(current, options)}>Save</button>` : nothing}
          ${hasCanonicalState ? html`<button type="button" class="text-button saved-actions-toggle" aria-expanded=${String(options.actionPanel() === 'save')} aria-controls="saved-query-form" @click=${() => options.onToggleActions('save')}>${current ? 'Save as…' : 'Save'}</button>` : nothing}
          ${current ? html`<button type="button" class="text-button saved-actions-toggle" aria-label="More saved exploration actions" title="More saved exploration actions" aria-expanded=${String(options.actionPanel() === 'more')} aria-controls="saved-version-actions" @click=${() => options.onToggleActions('more')}>${lucideIcon(Ellipsis, { size: 18 })}</button>` : nothing}
        </div>
        ${hasCanonicalState ? html`<div id="saved-query-form" class="saved-exploration-actions" role="group" aria-label=${current ? 'Save current query as a copy' : 'Save exploration'} ?hidden=${options.actionPanel() !== 'save'}>
          <label>Name<input type="text" aria-label=${current ? 'Current query name' : 'Saved exploration name'} placeholder=${options.suggestedTitle?.() || suggestedExplorationTitle(options.activeSpec())} .value=${options.savedTitle()} @input=${(event: Event) => options.onSavedTitleInput((event.target as HTMLInputElement).value)} /></label>
          ${current ? nothing : savedVisibilitySelect(options.savedVisibility(), options.onSavedVisibilityInput, saving)}
          <button type="button" class="text-button" ?disabled=${saving} aria-label=${current ? 'Save current query as a copy' : 'Save current exploration'} @click=${() => current ? saveCurrentQuery(options) : createSavedExploration(options)}>${current ? 'Save copy' : 'Save'}</button>
        </div>` : nothing}
        ${current ? html`<div id="saved-version-actions" class="saved-exploration-actions" role="group" aria-label=${`Saved exploration actions for ${current.title}`} ?hidden=${options.actionPanel() !== 'more'}>
          ${current.status === 'archived' ? html`<span class="saved-exploration-status">Archived · read only</span>` : nothing}
          <button type="button" class="text-button" @click=${() => options.onReopen(current)}>Reopen saved version</button>
          ${current.canEdit && current.status === 'active' && current.detached ? html`${savedVisibilitySelect(options.currentSavedVisibility(current), options.onCurrentSavedVisibilityInput, saving)}<button type="button" class="text-button" ?disabled=${saving} @click=${() => saveSavedExploration(current, options)}>Save changes</button>` : nothing}
          <label>Copy name<input type="text" aria-label="Duplicate saved exploration name" placeholder=${`Copy of ${current.title}`} .value=${options.savedDuplicateTitle()} @input=${(event: Event) => options.onDuplicateTitleInput((event.target as HTMLInputElement).value)} /></label>
          <button type="button" class="text-button" ?disabled=${saving} @click=${() => duplicateSavedExploration(current, options)}>Duplicate saved version</button>
          ${current.canArchive && current.status === 'active' ? html`<button type="button" class="text-button archive-action" ?disabled=${unsaved || saving} title=${unsaved ? 'Save your changes before archiving' : 'Archive saved exploration'} @click=${() => archiveSavedExploration(current, options)}>Archive</button>` : nothing}
        </div>` : nothing}
      `}
    </section>
  `
}

export function renderExplorationShareMenu(state: SavedExplorationStateSignal, options: SavedExplorationViewOptions) {
  if (!state.enabled && !options.dashboardAppend) return nothing
  const currentQueryURL = options.canSaveCurrent() && options.activeSpec().modelId?.trim()
    ? dataExplorerURL({ mode: 'explore', explore: { spec: options.activeSpec() } } as DataExplorerCommand)
    : ''
  const savedURL = state.current ? savedExplorationShareURL(state.current.id, state.current.status === 'archived') : ''
  if (!currentQueryURL && !savedURL && !options.dashboardAppend) return nothing
  return html`
    <details class="saved-exploration-sharing" @keydown=${(event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      const menu = event.currentTarget as HTMLDetailsElement
      menu.open = false
      menu.querySelector<HTMLElement>(':scope > summary')?.focus()
      event.stopPropagation()
    }}>
      <summary aria-label="Share or export exploration" @click=${(event: Event) => {
        if (!(event.currentTarget as HTMLElement).closest('details')?.open) options.onShareStatus('', '')
      }}>${lucideIcon(Share2, { size: 15 })}<span>Share</span></summary>
      <div class="saved-exploration-sharing-actions">
        <span class="saved-exploration-sharing-label">Share</span>
        ${currentQueryURL ? html`<button type="button" class="text-button" @click=${() => void copyExplorationLink(currentQueryURL, options)}>${lucideIcon(Link, { size: 16 })}Copy current query link</button>` : nothing}
        ${savedURL ? html`<button type="button" class="text-button" @click=${() => void copyExplorationLink(savedURL, options)}>${lucideIcon(Link, { size: 16 })}Copy saved version link</button>` : nothing}
        ${options.dashboardAppend?.() ?? nothing}
        ${currentQueryURL ? html`
          <span class="saved-exploration-sharing-label export-label">Export</span>
          <a class="saved-exploration-download" @click=${(event: MouseEvent) => { event.preventDefault(); void downloadExploration((event.currentTarget as HTMLAnchorElement).href, options) }} href=${dataExplorerExportURL({ mode: 'explore', explore: { spec: options.activeSpec() } } as DataExplorerCommand, 'csv')}>${lucideIcon(Download, { size: 16 })}Download CSV</a>
          <a class="saved-exploration-download" @click=${(event: MouseEvent) => { event.preventDefault(); void downloadExploration((event.currentTarget as HTMLAnchorElement).href, options) }} href=${dataExplorerExportURL({ mode: 'explore', explore: { spec: options.activeSpec() } } as DataExplorerCommand, 'parquet')}>${lucideIcon(Download, { size: 16 })}Download Parquet</a>
          <span class="saved-exploration-export-unavailable">Export complete results up to 10,000 rows and 32 MiB. Add filters for larger results.</span>
        ` : nothing}
        <span class="saved-exploration-sharing-hint">Links run live data with the viewer’s access.</span>
        ${options.shareStatus() ? html`<span role="status" aria-live="polite">${options.shareStatus()}</span>` : nothing}
        ${options.shareFallbackURL() ? html`<a class="saved-exploration-share-fallback" href=${options.shareFallbackURL()}>Open link</a>` : nothing}
      </div>
    </details>
  `
}

export async function copyExplorationLink(path: string, options: Pick<SavedExplorationViewOptions, 'onShareStatus'>): Promise<void> {
  const url = new URL(path, window.location.origin).href
  try {
    await navigator.clipboard.writeText(url)
    options.onShareStatus('Link copied.', '')
  } catch {
    options.onShareStatus('Clipboard unavailable. Open the link directly.', url)
  }
}

function savedVisibilitySelect(
  value: SavedExplorationVisibility,
  onChange: (value: SavedExplorationVisibility) => void,
  disabled = false,
) {
  return html`<label>Visibility <select aria-label="Saved exploration visibility" .value=${value} ?disabled=${disabled} @change=${(event: Event) => onChange((event.target as HTMLSelectElement).value as SavedExplorationVisibility)}><option value="private">Personal</option><option value="organization">Organization</option></select></label>`
}

function saveSavedExploration(current: SavedExplorationCurrent, options: SavedExplorationViewOptions): void {
  if (!current.canEdit) return
  const activeSpec = options.activeSpec()
  const spec = activeSpec.modelId?.trim() ? activeSpec : (current.spec ?? activeSpec)
  options.onCommand({ action: 'update', explorationId: current.id, title: current.title, slug: current.slug, visibility: options.currentSavedVisibility(current), spec, expectedRevision: current.revision })
}

function createSavedExploration(options: SavedExplorationViewOptions): void {
  const activeSpec = options.activeSpec()
  const title = options.savedTitle().trim() || options.suggestedTitle?.() || suggestedExplorationTitle(activeSpec)
  options.onCommand({ action: 'create', title, visibility: options.savedVisibility(), spec: activeSpec })
}

function saveCurrentQuery(options: SavedExplorationViewOptions): void {
  const activeSpec = options.activeSpec()
  const title = options.savedTitle().trim() || options.suggestedTitle?.() || suggestedExplorationTitle(activeSpec)
  options.onCommand({ action: 'create', title, visibility: 'private', spec: activeSpec })
}

function duplicateSavedExploration(current: SavedExplorationCurrent, options: SavedExplorationViewOptions): void {
  const title = options.savedDuplicateTitle().trim() || `Copy of ${current.title}`
  options.onCommand({ action: 'duplicate', sourceExplorationId: current.id, title, visibility: current.visibility, expectedSourceRevision: current.revision })
}

function archiveSavedExploration(current: SavedExplorationCurrent, options: SavedExplorationViewOptions): void {
  if (!current.canArchive) return
  options.onCommand({ action: 'archive', explorationId: current.id, expectedRevision: current.revision })
}

export function savedExplorationSelectionIncludesArchived(state: SavedExplorationStateSignal): boolean {
  const selected = state.list?.selectedId?.trim() ?? ''
  if (state.current?.status === 'archived' && (!selected || state.current.id === selected)) return true
  return (state.list?.items ?? []).some((item) => item.id === selected && item.status === 'archived')
}

export async function downloadExploration(url: string, options: Pick<SavedExplorationViewOptions, 'onShareStatus'>): Promise<void> {
  options.onShareStatus('Preparing download…', '')
  try {
    const response = await fetch(url, { credentials: 'same-origin' })
    if (!response.ok) {
      if (response.status === 413) throw new Error('This export exceeds 10,000 rows or 32 MiB. Open Filters to narrow the query, then download again. Your exploration is unchanged.')
      throw new Error('Could not download this exploration. Review your access and try again.')
    }
    const contentType = response.headers.get('Content-Type')?.split(';')[0]?.trim().toLowerCase()
    if (response.redirected || !['text/csv', 'application/vnd.apache.parquet'].includes(contentType ?? '')) {
      throw new Error('Download access could not be confirmed. Sign in again, then retry from this exploration.')
    }
    const blob = await response.blob()
    const objectURL = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = objectURL
    link.download = new URL(url, window.location.href).searchParams.get('format') === 'parquet' || url.includes('/parquet') ? 'exploration.parquet' : 'exploration.csv'
    document.body.append(link)
    link.click()
    link.remove()
    setTimeout(() => URL.revokeObjectURL(objectURL), 60_000)
    options.onShareStatus('Download ready.', '')
  } catch (error) {
    options.onShareStatus(error instanceof Error ? error.message : 'Download failed. Try again.', '')
  }
}
