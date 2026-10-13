import { LitElement, css, html, nothing } from 'lit'
import { state } from 'lit/decorators.js'
import { settingsLayoutStyles } from '../web/components/shared/settings-layout'
import overlay from '../scripts/reference-library/overlay.json'
import project from '../package.json'

type Source = { id: string; title: string; kind: string; package?: string; module?: string; owner: string; terms: string[]; topics: string[]; capabilities: string[] }
type Change = { id: string; title: string; change: string; version: string; owner: string; terms: string[]; use_when_added?: string[] }
const requirements: Record<string, string> = { ...project.dependencies, ...project.devDependencies }
const titles: Record<string, string> = { echarts: 'Apache ECharts', 'tanstack-table': 'TanStack Table', 'tanstack-virtual': 'TanStack Virtual', 'maplibre-gl-js': 'MapLibre GL JS', duckdb: 'DuckDB', ducklake: 'DuckLake' }
const changes: Change[] = [
  ...(overlay.sources as Source[]).map(source => ({ ...source, change: 'added', version: source.package ? requirements[source.package] || 'Not declared' : source.kind === 'go' ? 'go.mod' : source.kind === 'project' ? 'Git HEAD' : 'Vendored runtime' })),
  ...Object.entries(overlay.enhancements).map(([id, guidance]) => ({ id, title: titles[id] || id, change: 'expanded guidance', version: '', owner: '', terms: guidance.terms, use_when_added: guidance.use_when })),
]
const report = {
  catalog_before: 52,
  catalog_after: 52 + overlay.sources.length,
  sources_added: overlay.sources.length,
  existing_guidance_updated: Object.keys(overlay.enhancements).length,
  limits: [
    'The maintenance script reads your installed catalog and measures the local files before making changes.',
    'Added references come from installed packages, cached Go modules, vendored Datastar and tracked LeapView source.',
    'Existing upstream snapshots and their acquisition metadata are preserved.',
    'Installation switches the local current symlink after validation; the previous release remains available for rollback.',
  ],
}

/** Review the canonical maintenance overlay without backend or filesystem access. */
class PlaygroundReferenceRefresh extends LitElement {
  @state() private showUpdate = true
  @state() private search = ''

  getExampleState() { return { showUpdate: this.showUpdate, search: this.search } }

  restoreExampleState(value: Record<string, unknown>) {
    if (typeof value.showUpdate === 'boolean') this.showUpdate = value.showUpdate
    if (typeof value.search === 'string') this.search = value.search.slice(0, 500)
  }

  getExampleCode() { return 'python3 scripts/reference_library.py plan\npython3 scripts/reference_library.py install' }

  static styles = [settingsLayoutStyles, css`
    :host { display:block; color:var(--lv-fg-default); font:var(--lv-type-body); }
    * { box-sizing:border-box; }
    article { padding:24px; background:var(--lv-bg-panel); border:var(--lv-border-muted); border-radius:var(--lv-radius-large); }
    h1 { margin:0 0 8px; font:var(--lv-type-page-title); }
    h2 { margin:24px 0 12px; font:var(--lv-type-section-title); }
    p { margin:8px 0; line-height:1.5; }
    .muted { color:var(--lv-fg-muted); }
    .eyebrow { font:var(--lv-type-caption); color:var(--lv-fg-accent); margin-bottom:12px; }
    .toolbar { display:flex; flex-wrap:wrap; gap:8px; margin:20px 0; align-items:center; }
    button[aria-pressed="true"] { border-color:var(--lv-fg-accent); color:var(--lv-fg-accent); }
    .stats { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:12px; }
    .stat { padding:16px; border:var(--lv-border-muted); border-radius:var(--lv-radius-default); background:var(--lv-bg-panel-muted); }
    .stat strong { display:block; font-size:28px; line-height:1.3; }
    .stat span { color:var(--lv-fg-muted); font:var(--lv-type-caption); }
    .notice { padding:12px 16px; margin:16px 0; border-left:3px solid var(--lv-fg-accent); background:var(--lv-bg-panel-muted); }
    .table-wrap { overflow:auto; }
    table { border-collapse:collapse; width:100%; font:var(--lv-type-caption); }
    th,td { text-align:left; vertical-align:top; padding:10px 12px; border-bottom:var(--lv-border-muted); }
    th { color:var(--lv-fg-muted); font-weight:600; }
    td:first-child { font-weight:600; }
    code { overflow-wrap:anywhere; }
    ul { line-height:1.6; padding-left:20px; }
    .sources { display:flex; gap:12px; flex-wrap:wrap; margin-top:12px; }
    a { color:var(--lv-fg-accent); }
    @media(max-width:600px) { article { padding:16px; } .stats { grid-template-columns:1fr; } }
  `]

  render() {
    const added = changes.filter(change => change.change === 'added')
    const improved = changes.filter(change => change.change !== 'added')
    const shown = added.filter(change => JSON.stringify(change).toLowerCase().includes(this.search.toLowerCase()))
    return html`<article aria-label="Flid reference library local review">
      <div class="eyebrow">FLID REFERENCE LIBRARY · UPDATE PLAN</div>
      <h1>${this.showUpdate ? 'References for the work we ship today' : 'September 8 reference coverage'}</h1>
      <p class="muted">Review the references used for LeapView components, AI tools, and browser checks.</p>
      <div class="toolbar" aria-label="Compare reference catalogs">
        <button class="settings-button" aria-pressed=${String(!this.showUpdate)} @click=${() => { this.showUpdate = false }}>Before update</button>
        <button class="settings-button" aria-pressed=${String(this.showUpdate)} @click=${() => { this.showUpdate = true }}>After update</button>
      </div>
      <div class="stats">
        <div class="stat" role="group" aria-label="Reference entry count"><strong>${this.showUpdate ? report.catalog_after : report.catalog_before}</strong><span>Searchable reference entries</span></div>
        <div class="stat"><strong>${this.showUpdate ? report.sources_added : 0}</strong><span>${this.showUpdate ? 'Added local references' : 'Entries for these missing dependencies'}</span></div>
        <div class="stat"><strong>${this.showUpdate ? report.existing_guidance_updated : 0}</strong><span>Existing references with expanded guidance</span></div>
      </div>
      <div class="notice">This example does not read your installed library. It compares the September 8 catalog's ${report.catalog_before} entries with the canonical overlay's ${report.sources_added} additions. Run the maintenance script to inspect or update the actual library.</div>
      ${this.showUpdate ? html`
        <h2>${report.sources_added} added references</h2>
        <label>Filter new references <input class="settings-input" type="search" .value=${this.search} @input=${(event: Event) => { this.search = (event.target as HTMLInputElement).value }} placeholder="Try chat, lineage, or Playground"></label>
        <p class="muted">${shown.length} of ${added.length} additions · requirements come from package.json; the installer resolves exact versions from local files.</p>
        <div class="table-wrap"><table aria-label="Added references"><thead><tr><th scope="col">Reference</th><th scope="col">Project dependency</th><th scope="col">Implementation evidence</th></tr></thead>
          <tbody>${shown.map(change => html`<tr><td>${change.title}</td><td><code>${change.version?.slice(0, 16)}</code></td><td><code>${change.owner}</code></td></tr>`)}</tbody>
        </table></div>
        ${shown.length === 0 ? html`<p role="status">No references match your search.</p>` : nothing}
        <h2>${report.existing_guidance_updated} existing references updated</h2>
        <div class="table-wrap"><table aria-label="Updated guidance"><thead><tr><th scope="col">Reference</th><th scope="col">New lookup guidance</th></tr></thead>
          <tbody>${improved.map(change => html`<tr><td>${change.title}</td><td>${change.use_when_added?.join('; ')}</td></tr>`)}</tbody>
        </table></div>
      ` : html`
        <h2>Coverage to add</h2>
        <p>${added.map(change => change.title).join(' · ')}</p>
        <h2>Why update now</h2>
        <ul><li>AI chat, MCP tools, signal delivery and assistant handoffs need direct implementation references.</li><li>Hierarchy tables, responsive charts and lineage exploration need current lookup guidance.</li><li>Playground browser review, accessibility checks and dashboard YAML contracts need discoverable tooling references.</li><li>First-party source connects deployment, recovery, typed authorization and pipeline work to the product.</li></ul>
      `}
      <h2>Review boundaries</h2>
      <ul>${report.limits.map(limit => html`<li>${limit}</li>`)}</ul>
      <p class="muted">Local package files are identified separately from full upstream mirrors. The script records their origin and content checksums without inventing upstream fetch dates.</p>
      <div class="sources"><a href="#content/assistant-handoffs">Assistant handoffs</a><a href="#graphs/asset-lineage">Asset lineage</a><a href="#recipes/dashboard-contract">Dashboard YAML contract</a></div>
    </article>`
  }
}

customElements.define('playground-reference-refresh', PlaygroundReferenceRefresh)
