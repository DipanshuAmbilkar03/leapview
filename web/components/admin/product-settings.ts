import { LitElement, css, html, nothing } from 'lit'
import { state } from 'lit/decorators.js'
import { ifDefined } from 'lit/directives/if-defined.js'
import { DatastarLit } from '../shared/datastar-lit'
import { browserCommandFailure } from '../shared/command-failure'
import { leapViewBrandName } from '../shared/brand-mark'
import { renderSettingsActions, renderSettingsRow, renderSettingsSection, settingsLayoutStyles } from '../shared/settings-layout'
import { settingsFieldStyles } from '../shared/settings-field-styles'
import type {
  ProductAPIStatusSignal,
  ProductAuthenticationSignal,
  ProductAvailabilitySignal,
  ProductGeneralSignal,
  ProductSettingsCommand,
  ProductSettingsSignal,
  ProductSystemSignal,
} from '../../generated/signals'

type ProductSection = 'general' | 'authentication' | 'system'
type StatusTone = 'positive' | 'neutral' | 'warning' | 'negative'
/** Where a command's outcome is reported: beside the control that caused it. */
type FeedbackTarget = 'name' | 'logo' | 'identity'
type Feedback = { target: FeedbackTarget; tone: 'error' | 'success'; message: string }

const logoMaxBytes = 5 * 1024 * 1024
const logoMediaTypes = ['image/jpeg', 'image/png', 'image/webp']

const emptyProductSettings: ProductSettingsSignal = {
  active: 'general', canManage: false,
  general: { displayName: '', revision: 0, updatedAt: '', instanceId: '', canonicalOrigin: '', environment: '' },
  authentication: {
    browserEnabled: false, apiTokenOnly: false,
    local: { available: false, enabled: false }, oidc: { available: false, enabled: false },
    azure: { available: false, enabled: false }, scim: { available: false, enabled: false }, managedBy: 'deployment',
  },
  api: {
    bearerCredentials: { available: false, enabled: false }, servicePrincipals: { available: false, enabled: false },
    oauth: { available: false, enabled: false }, mcp: { available: false, enabled: false }, externalMcpIssuer: false,
  },
  system: {
    instanceId: '', canonicalOrigin: '', environment: '',
    build: { version: '', revision: '', buildTime: '', dirty: false, development: false },
    storageBackend: '', agent: { available: false, configured: false, modelConfigured: false },
    limits: { queryResultMaxRows: 0, queryResultMaxBytes: 0, managedDataMaxFiles: 0, managedDataMaxFileBytes: 0, managedDataMaxRevisionBytes: 0 },
    runtime: { health: '', controlPlane: '', environment: '' },
  },
}

/**
 * Platform settings rendered from the page-stream `productSettings` signal.
 * Text edits and destructive actions emit typed commands; only a binary logo
 * body uses the same-origin browser upload route.
 */
export class LeapViewProductSettings extends DatastarLit(LitElement) {
  @state() private selectedSection: ProductSection = 'general'
  @state() private displayNameDraft = ''
  @state() private busy = false
  @state() private commandBusy = false
  @state() private commandError = ''
  @state() private feedback: Feedback | null = null
  private lastRevision = -1
  private pendingRevision = -1
  private pendingError = ''
  private pendingAction: ProductSettingsCommand['action'] | '' = ''

  override connectedCallback(): void {
    super.connectedCallback()
    document.addEventListener('datastar-fetch', this.handleDatastarFetch)
  }

  override disconnectedCallback(): void {
    document.removeEventListener('datastar-fetch', this.handleDatastarFetch)
    super.disconnectedCallback()
  }

  static styles = [settingsFieldStyles, settingsLayoutStyles, css`
    :host { display: block; min-width: 0; color: var(--lv-fg-default); font: var(--lv-type-body); }
    .hint { color: var(--lv-fg-muted); font: var(--lv-type-caption); line-height: var(--base-text-lineHeight-snug); }
    .logo { display: flex; align-items: center; gap: .75rem; }
    .logo img { width: 3.25rem; height: 3.25rem; border: var(--lv-border-muted); border-radius: var(--lv-radius-small); object-fit: contain; background: var(--lv-bg-panel); }
    .identity-preview { display: flex; min-width: 0; align-items: center; gap: var(--base-size-12); border: var(--lv-border-muted); border-radius: var(--lv-radius-default); background: var(--lv-bg-panel-muted); padding: var(--base-size-12); }
    .identity-preview img { box-sizing: border-box; display: grid; width: var(--control-large-size); height: var(--control-large-size); flex: 0 0 auto; place-items: center; border: var(--lv-border-muted); border-radius: var(--lv-radius-small); background: var(--lv-bg-panel); object-fit: contain; }
    .identity-copy { display: grid; min-width: 0; gap: var(--base-size-2); }
    .identity-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: var(--lv-type-body); font-weight: var(--base-text-weight-semibold); }
    .file-action { position: relative; }
    .file-action input { position: absolute; width: 1px; height: 1px; opacity: 0; overflow: hidden; }
    .about-links { display: flex; flex-wrap: wrap; gap: var(--base-size-16); }
    .about-links a { color: var(--lv-fg-accent); }
    .status-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(12rem, 100%), 1fr)); gap: var(--base-size-8); }
    .status-card { display: grid; min-width: 0; gap: var(--base-size-4); border: var(--lv-border-muted); border-radius: var(--lv-radius-small); padding: var(--base-size-12); }
    .status-card strong { font: var(--lv-type-body); font-weight: var(--base-text-weight-semibold); }
    .status { overflow-wrap: anywhere; font: var(--lv-type-caption); }
    .status-positive, .status.enabled { color: var(--lv-fg-success); }
    .status-neutral, .status.disabled { color: var(--lv-fg-muted); }
    .status-warning { color: var(--lv-fg-warning); }
    .status-negative { color: var(--lv-fg-danger); }
    .notice { border-radius: var(--lv-radius-small); background: var(--lv-bg-panel-muted); color: var(--lv-fg-muted); padding: var(--base-size-8) var(--base-size-12); font: var(--lv-type-caption); }
    .field-stack { display: grid; min-width: 0; gap: var(--base-size-8); }
    .field-feedback { margin: 0; overflow-wrap: anywhere; font: var(--lv-type-caption); }
    .field-feedback[data-tone="error"] { color: var(--lv-fg-danger); }
    .field-feedback[data-tone="success"] { color: var(--lv-fg-success); }
    .settings-input[aria-invalid="true"] { border-color: var(--lv-line-danger); }
    @media (max-width: 480px) {
      .status-grid { grid-template-columns: 1fr; }
      .logo { align-items: flex-start; }
    }
  `]

  private get settings(): ProductSettingsSignal {
    return this.signal<ProductSettingsSignal>('productSettings', emptyProductSettings)
  }

  override updated(): void {
    const settings = this.settings
    const revision = settings.general.revision
    if (revision !== this.lastRevision) {
      this.lastRevision = revision
      this.displayNameDraft = this.settings.general.displayName
    }
    if (this.commandBusy && (revision !== this.pendingRevision || (settings.error ?? '') !== this.pendingError)) {
      this.commandBusy = false
      this.settleCommand(settings)
    }
    const active = settings.active as ProductSection
    if ((active === 'general' || active === 'authentication' || active === 'system') && active !== this.selectedSection) this.selectedSection = active
  }

  render() {
    const settings = this.settings
    return html`
      <div class="settings-stack" aria-label="Product settings">
        ${settings.error && !this.feedback ? html`<div class="notice" role="alert">${settings.error}</div>` : nothing}
        ${this.selectedSection === 'general' ? this.renderGeneral(settings.general, settings.canManage) : nothing}
        ${this.selectedSection === 'authentication' ? this.renderAuthentication(settings.authentication, settings.api) : nothing}
        ${this.selectedSection === 'system' ? this.renderSystem(settings.system) : nothing}
        ${this.commandError ? html`<div class="notice" role="alert">${this.commandError}</div>` : nothing}
      </div>
    `
  }

  private renderGeneral(general: ProductGeneralSignal, canManage: boolean) {
    const previewName = this.displayNameDraft.trim() || leapViewBrandName
    const disabled = this.busy || this.commandBusy
    // Keep reset available while a name mutation is pending so a user can
    // choose the explicit restore-default action; the in-flight command is
    // still serialized by the page command transport and all other controls
    // remain disabled until its signal patch or failure arrives.
    const nameError = this.nameValidationError(general)
    const resetDisabled = !canManage || this.busy || (general.displayName === leapViewBrandName && !general.logo)
    return html`
      ${renderSettingsSection({ label: 'Instance identity settings', className: 'panel', appearance: 'panel', heading: 'Instance identity', description: 'Choose the name and logo shown in the application.', content: html`
        <div class="identity-preview" aria-label="Instance identity preview">
          ${general.logo ? html`<img src=${general.logo.url} alt="">` : nothing}
          <span class="identity-copy">
            <span class="identity-name">${previewName}</span>
          </span>
        </div>
        <div class="settings-rows">
          ${renderSettingsRow({ label: 'Instance name', layout: 'field', className: 'row', controlId: 'product-instance-name', description: 'Used in navigation and browser titles · 120 characters maximum', control: html`${renderSettingsActions(html`
              <input class="settings-input" id="product-instance-name" aria-label="Instance name" type="text" maxlength="120" aria-invalid=${nameError || (this.feedback?.target === 'name' && this.feedback.tone === 'error') ? 'true' : 'false'} aria-describedby="product-instance-name-feedback" .value=${this.displayNameDraft} ?disabled=${!canManage || disabled} @input=${this.handleDisplayNameInput}>
              <button class="settings-button action primary" type="button" ?disabled=${!canManage || disabled || Boolean(nameError) || this.displayNameDraft.trim() === general.displayName} @click=${this.saveDisplayName}>Save</button>
            `, { className: 'inline', stack: true })}${this.renderFeedback('name', 'product-instance-name-feedback', nameError)}` })}
          ${renderSettingsRow({ label: 'Instance logo', layout: 'field', className: 'row', description: 'JPEG, PNG, or WebP · 5 MB maximum', control: html`<div class="field-stack">${renderSettingsActions(html`
              ${general.logo ? html`<div class="logo"><img src=${general.logo.url} alt="Product logo"><span class="hint">${general.logo.width} × ${general.logo.height}</span></div>` : html`<span class="settings-value">No logo configured</span>`}
              <label class=${`settings-button file-action ${!canManage || disabled ? 'disabled' : ''}`}>
                <span>${general.logo ? 'Change logo' : 'Upload logo'}</span>
                <input id="product-logo-upload" aria-label=${general.logo ? 'Change logo' : 'Upload logo'} aria-describedby="product-logo-feedback" type="file" accept="image/jpeg,image/png,image/webp" ?disabled=${!canManage || disabled} @change=${this.handleLogoFile}>
              </label>
              ${general.logo ? html`<button class="settings-button action danger" type="button" ?disabled=${!canManage || disabled} @click=${this.removeLogo}>Remove</button>` : nothing}
            `, { className: 'inline', stack: true })}${this.renderFeedback('logo', 'product-logo-feedback')}</div>` })}
          ${renderSettingsRow({ label: 'LeapView defaults', layout: 'field', className: 'row', description: 'Restore the default name and remove the custom logo', control: html`<div class="field-stack"><button class="settings-button action" type="button" ?disabled=${resetDisabled} @click=${this.resetIdentity}>Reset to ${leapViewBrandName}</button>${this.renderFeedback('identity', 'product-identity-feedback')}</div>` })}
        </div>
        ${!canManage ? html`<div class="notice">You have read-only access. Platform administrator access is required to change product identity.</div>` : nothing}
      ` })}
      ${renderSettingsSection({ label: 'Instance details', className: 'panel', appearance: 'panel', heading: 'Instance details', description: 'Read-only deployment metadata for this installation.', content: html`
        <div class="settings-rows">
          ${this.settingsRow('Instance ID', general.instanceId || 'Unknown')}
          ${this.settingsRow('Canonical origin', general.canonicalOrigin || 'Unknown')}
          ${this.settingsRow('Environment', general.environment || 'Unknown')}
          ${this.settingsRow('Last updated', `${general.updatedAt || 'Unknown'} · revision ${general.revision || 'unknown'}`)}
        </div>
      ` })}
    `
  }

  private renderAuthentication(auth: ProductAuthenticationSignal, api: ProductAPIStatusSignal) {
    return html`
      ${renderSettingsSection({ label: 'Sign-in and provisioning settings', className: 'panel', appearance: 'panel', heading: 'Sign-in & provisioning', description: 'Review deployment-managed sign-in and provisioning capabilities. Secrets, issuer URLs, tenant IDs, and callback URLs are never exposed here.', content: html`
        <div class="notice">Managed by <strong>${auth.managedBy || 'deployment'}</strong>; configuration changes are made through deployment settings.</div>
        <div class="status-grid">
          ${this.statusCard('Browser sign-in', statusTone(auth.browserEnabled), auth.browserEnabled ? 'Enabled' : 'Disabled')}
          ${this.statusCard('API-token-only mode', statusTone(auth.apiTokenOnly), auth.apiTokenOnly ? 'Enabled' : 'Disabled')}
          ${this.statusCard('Local credentials', availabilityTone(auth.local), availabilityLabel(auth.local))}
          ${this.statusCard('OIDC', availabilityTone(auth.oidc), `${availabilityLabel(auth.oidc)}${auth.oidc.provider ? ` · ${auth.oidc.provider}` : ''}`)}
          ${this.statusCard('Azure', availabilityTone(auth.azure), availabilityLabel(auth.azure))}
          ${this.statusCard('SCIM provisioning', availabilityTone(auth.scim), availabilityLabel(auth.scim))}
        </div>
      ` })}
      ${renderSettingsSection({ label: 'API and protocols settings', className: 'panel', appearance: 'panel', heading: 'API & protocols', description: 'Review deployment-managed API credentials, service identities, and protocol endpoints.', content: html`
        <div class="status-grid">
          ${this.statusCard('Bearer credentials', availabilityTone(api.bearerCredentials), availabilityLabel(api.bearerCredentials))}
          ${this.statusCard('Service principals', availabilityTone(api.servicePrincipals), availabilityLabel(api.servicePrincipals))}
          ${this.statusCard('OAuth', availabilityTone(api.oauth), availabilityLabel(api.oauth))}
          ${this.statusCard('MCP', availabilityTone(api.mcp), availabilityLabel(api.mcp))}
          ${this.statusCard('External MCP issuer', statusTone(api.externalMcpIssuer), api.externalMcpIssuer ? 'Configured' : 'Not configured')}
        </div>
      ` })}
    `
  }

  private renderSystem(system: ProductSystemSignal) {
    const build = system.build
    const limits = system.limits
    const agent = system.agent
    return html`
      ${renderSettingsSection({ label: 'Runtime health settings', className: 'panel', appearance: 'panel', heading: 'Runtime health', description: 'Runtime health and safe operational metadata for this instance.', content: html`
        <div class="status-grid">
          ${this.statusCard('Runtime health', runtimeTone(system.runtime.health), system.runtime.health || 'Unknown')}
          ${this.statusCard('Control plane', runtimeTone(system.runtime.controlPlane, 'available'), system.runtime.controlPlane || 'Unknown')}
          ${this.statusCard('Agent configuration', agentTone(agent), agent.configured ? `${agent.provider || 'Configured'}${agent.modelConfigured ? ' · model configured' : ' · model missing'}` : 'Not configured')}
          ${this.statusCard('Storage backend', system.storageBackend ? 'positive' : 'neutral', system.storageBackend || 'Unknown')}
        </div>
        <p class="muted">Configuration does not verify provider readiness. <a href="/admin/agent">View Agent diagnostics</a>.</p>
        <div class="settings-rows">
          ${this.settingsRow('Instance ID', system.instanceId || 'Unknown')}
          ${this.settingsRow('Canonical origin', system.canonicalOrigin || 'Unknown')}
          ${this.settingsRow('Environment', system.environment || 'Unknown')}
        </div>
      ` })}
      ${renderSettingsSection({ label: 'Build settings', className: 'panel', appearance: 'panel', heading: 'Build', description: 'Version and release metadata reported by the running instance.', content: html`
        <div class="status-grid">
          ${this.statusCard('Version', build.version ? 'neutral' : 'warning', build.version || 'Unknown')}
          ${this.statusCard('Revision', build.revision ? 'neutral' : 'warning', build.revision || 'Unknown')}
          ${this.statusCard('Build time', build.buildTime ? 'neutral' : 'warning', build.buildTime || 'Unknown')}
          ${this.statusCard('Build state', build.dirty ? 'negative' : build.development ? 'warning' : 'positive', build.dirty ? 'Dirty' : build.development ? 'Development' : 'Release')}
        </div>
      ` })}
      ${renderSettingsSection({ label: 'Limits settings', className: 'panel', appearance: 'panel', heading: 'Limits', description: 'Configured query and managed-data budgets for this instance.', content: html`
        <div class="status-grid">
          ${this.limitCard('Query result rows', limits.queryResultMaxRows, 'count')}
          ${this.limitCard('Query result bytes', limits.queryResultMaxBytes)}
          ${this.limitCard('Managed-data files', limits.managedDataMaxFiles, 'count')}
          ${this.limitCard('Managed-data file bytes', limits.managedDataMaxFileBytes)}
          ${this.limitCard('Managed-data revision bytes', limits.managedDataMaxRevisionBytes)}
        </div>
      ` })}
      ${renderSettingsSection({ label: 'About LeapView', className: 'panel', appearance: 'panel', content: html`
        <div class="panel-heading">
          <h2>About LeapView</h2>
          <p>Powered by LeapView, open-source dashboards-as-code business intelligence.</p>
        </div>
        <div class="about-links">
          <a href="https://leapview.dev" target="_blank" rel="noreferrer">LeapView website</a>
          <a href="https://github.com/flidai/leapview" target="_blank" rel="noreferrer">View source</a>
        </div>
        <p class="hint">Build ${build.version || 'development'}${build.revision ? ` · ${build.revision}` : ''}</p>
      ` })}
    `
  }

  private settingsRow(label: string, value: string) {
    return renderSettingsRow({ label, control: html`<span class="settings-value">${value}</span>` })
  }

  private statusCard(label: string, tone: StatusTone, value: string) {
    return html`<div class="status-card"><strong>${label}</strong><span class=${`status status-${tone} ${tone === 'positive' ? 'enabled' : tone === 'neutral' ? 'disabled' : ''}`} data-status=${tone} role="status">${value}</span></div>`
  }

  private limitCard(label: string, value: number, format: 'bytes' | 'count' = 'bytes') {
    return html`<div class="status-card"><strong>${label}</strong><span class="settings-value">${format === 'count' ? formatCount(value) : formatLimit(value)}</span></div>`
  }

  /** A live region that stays mounted so assistive technology announces the outcome when it appears. */
  private renderFeedback(target: FeedbackTarget, id: string, validation = '') {
    const feedback = this.feedback?.target === target ? this.feedback : null
    const message = validation || feedback?.message || ''
    const tone = validation ? 'error' : feedback?.tone
    return html`<p class="field-feedback" id=${id} data-tone=${ifDefined(tone)} role=${tone === 'error' ? 'alert' : 'status'} ?hidden=${!message}>${message}</p>`
  }

  /** An empty or blank name would silently fall back to the default; ask for an explicit choice instead. */
  private nameValidationError(general: ProductGeneralSignal): string {
    if (this.displayNameDraft === general.displayName || this.displayNameDraft.trim() !== '') return ''
    return `Enter an instance name, or choose Reset to ${leapViewBrandName} to restore the default.`
  }

  private settleCommand(settings: ProductSettingsSignal): void {
    const target = feedbackTarget(this.pendingAction)
    const error = settings.error ?? ''
    if (!target) return
    if (error && error !== this.pendingError) this.feedback = { target, tone: 'error', message: error }
    else this.feedback = { target, tone: 'success', message: successMessage(this.pendingAction) }
  }

  private handleDisplayNameInput = (event: Event): void => {
    this.displayNameDraft = (event.currentTarget as HTMLInputElement).value
    if (this.feedback?.target === 'name') this.feedback = null
  }

  private saveDisplayName = (): void => {
    this.emitCommand({ action: 'save_display_name', displayName: this.displayNameDraft.trim(), revision: this.settings.general.revision })
  }

  private removeLogo = (): void => {
    this.emitCommand({ action: 'remove_logo', revision: this.settings.general.revision })
  }

  private resetIdentity = (): void => {
    this.emitCommand({ action: 'reset_identity', revision: this.settings.general.revision })
  }

  private emitCommand(command: ProductSettingsCommand): void {
    this.commandError = ''
    if (command.action !== 'refresh') {
      this.feedback = null
      this.pendingAction = command.action
      this.commandBusy = true
      this.pendingRevision = this.settings.general.revision
      this.pendingError = this.settings.error ?? ''
    }
    this.dispatchEvent(new CustomEvent<ProductSettingsCommand>('lv-product-settings-command', { bubbles: true, composed: true, detail: command }))
  }

  private handleDatastarFetch = (event: Event): void => {
    if (!this.commandBusy) return
    const failure = browserCommandFailure(event, 'Product settings update')
    if (!failure) return
    this.commandBusy = false
    const target = feedbackTarget(this.pendingAction)
    if (target) this.feedback = { target, tone: 'error', message: failure.message }
    else this.commandError = failure.message
  }

  private handleLogoFile = async (event: Event): Promise<void> => {
    const input = event.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    if (!file) return
    const settings = this.settings
    if (!settings.canManage) return
    const rejection = logoRejection(file)
    if (rejection) {
      this.feedback = { target: 'logo', tone: 'error', message: rejection }
      input.value = ''
      return
    }
    this.busy = true
    this.feedback = null
    try {
      const response = await fetch('/admin/product-logo', {
        method: 'PUT',
        credentials: 'same-origin',
        headers: {
          ...window.LeapViewCommand.headers('uploadProductLogo', productETag(settings.general.revision)),
          'Content-Type': file.type,
        },
        body: file,
      })
      if (!response.ok) throw new Error(await logoUploadFailure(response))
      this.feedback = { target: 'logo', tone: 'success', message: 'Logo uploaded.' }
      this.dispatchEvent(new CustomEvent('lv-product-settings-logo-updated', { bubbles: true, composed: true, detail: { etag: response.headers.get('ETag') ?? '' } }))
      this.emitCommand({ action: 'refresh', revision: 0 })
    } catch (error) {
      this.feedback = { target: 'logo', tone: 'error', message: error instanceof Error ? error.message : 'Logo upload failed. Try again.' }
    } finally {
      this.busy = false
      input.value = ''
    }
  }
}

function feedbackTarget(action: ProductSettingsCommand['action'] | ''): FeedbackTarget | '' {
  if (action === 'save_display_name') return 'name'
  if (action === 'remove_logo') return 'logo'
  if (action === 'reset_identity') return 'identity'
  return ''
}

function successMessage(action: ProductSettingsCommand['action'] | ''): string {
  if (action === 'save_display_name') return 'Instance name saved.'
  if (action === 'remove_logo') return 'Logo removed.'
  return `Identity reset to ${leapViewBrandName} defaults.`
}

/** Reject what the server would, before spending an upload on it. */
export function logoRejection(file: Pick<File, 'type' | 'size'>): string {
  if (!logoMediaTypes.includes(file.type)) return 'Use a JPEG, PNG, or WebP image.'
  if (file.size > logoMaxBytes) return 'The logo is larger than 5 MB. Choose a smaller image.'
  return ''
}

/** Prefer the server's explanation (problem+json detail); otherwise say what the status means. */
export async function logoUploadFailure(response: Pick<Response, 'status' | 'json'>): Promise<string> {
  try {
    const problem = await response.json() as { detail?: unknown }
    if (typeof problem.detail === 'string' && problem.detail.trim()) return problem.detail.trim()
  } catch {
    // Proxies and plaintext errors fall through to the status-based wording.
  }
  if (response.status === 413) return 'The logo is larger than 5 MB. Choose a smaller image.'
  if (response.status === 415 || response.status === 422) return 'The logo could not be used. Choose a JPEG, PNG, or WebP image up to 5 MB.'
  if (response.status === 409 || response.status === 412) return 'Settings changed in another session. Reload the page and try again.'
  if (response.status === 401 || response.status === 403) return 'You do not have permission to change the logo.'
  return `Logo upload failed (${response.status}). Try again.`
}

function availabilityLabel(status: ProductAvailabilitySignal): string {
  if (!status.available) return 'Unavailable'
  return status.enabled ? 'Enabled' : 'Disabled'
}

function statusTone(enabled: boolean): StatusTone {
  return enabled ? 'positive' : 'neutral'
}

function availabilityTone(status: ProductAvailabilitySignal): StatusTone {
  if (!status.available) return 'neutral'
  return status.enabled ? 'positive' : 'neutral'
}

function runtimeTone(value: string, healthyValue = 'healthy'): StatusTone {
  if (!value) return 'neutral'
  return value === healthyValue || value === 'available' ? 'positive' : 'negative'
}

function agentTone(agent: ProductSystemSignal['agent']): StatusTone {
  if (!agent.configured) return 'neutral'
  return agent.modelConfigured ? 'neutral' : 'warning'
}

function formatLimit(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return 'Not configured'
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(1)} GB`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)} MB`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)} KB`
  return value.toLocaleString()
}

function formatCount(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return 'Not configured'
  return value.toLocaleString()
}

function productETag(revision: number): string {
  return `"product-${revision}"`
}

if (!customElements.get('lv-product-settings')) customElements.define('lv-product-settings', LeapViewProductSettings)
