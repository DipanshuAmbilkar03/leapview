import { LitElement, css, html, nothing } from 'lit'
import { state } from 'lit/decorators.js'
import { settingsLayoutStyles } from '../web/components/shared/settings-layout'
import overlay from '../scripts/reference-library/overlay.json'
import project from '../package.json'

type Source = { id: string; title: string; kind: string; package?: string; owner: string; terms: string[] }
const requirements: Record<string, string> = { ...project.dependencies, ...project.devDependencies }
const titles: Record<string, string> = { echarts: 'Apache ECharts', 'tanstack-table': 'TanStack Table', 'tanstack-virtual': 'TanStack Virtual', 'maplibre-gl-js': 'MapLibre GL JS', duckdb: 'DuckDB', ducklake: 'DuckLake' }
const references = (overlay.sources as Source[]).map(source => ({
  ...source,
  requirement: source.package ? requirements[source.package] || 'Not declared' : source.kind === 'go' ? 'go.mod' : source.kind === 'project' ? 'Git HEAD' : 'Vendored runtime',
}))
const guidance = Object.entries(overlay.enhancements).map(([id, value]) => ({ title: titles[id] || id, topics: value.use_when }))

/** Browse the project's reference metadata. */
class PlaygroundReferenceRefresh extends LitElement {
  @state() private search = ''

  getExampleState() { return { search: this.search } }

  restoreExampleState(value: Record<string, unknown>) {
    if (typeof value.search === 'string') this.search = value.search.slice(0, 500)
  }

  static styles = [settingsLayoutStyles, css`
    :host { display:block; color:var(--lv-fg-default); font:var(--lv-type-body); }
    * { box-sizing:border-box; }
    article { padding:24px; background:var(--lv-bg-panel); border:var(--lv-border-muted); border-radius:var(--lv-radius-large); }
    h2 { margin:24px 0 12px; font:var(--lv-type-section-title); }
    h2:first-child { margin-top:0; }
    p { margin:8px 0; line-height:1.5; }
    .muted { color:var(--lv-fg-muted); }
    .table-wrap { overflow:auto; }
    table { border-collapse:collapse; width:100%; font:var(--lv-type-caption); }
    th,td { text-align:left; vertical-align:top; padding:10px 12px; border-bottom:var(--lv-border-muted); }
    th { color:var(--lv-fg-muted); font-weight:600; }
    td:first-child { font-weight:600; }
    code { overflow-wrap:anywhere; }
    @media(max-width:600px) { article { padding:16px; } }
  `]

  render() {
    const shown = references.filter(source => JSON.stringify(source).toLowerCase().includes(this.search.toLowerCase()))
    return html`<article part="preview" aria-label="Flid reference library">
      <h2>Reference library</h2>
      <p class="muted">Libraries and implementation references used by LeapView.</p>
      <label>Filter references <input class="settings-input" type="search" maxlength="500" .value=${this.search} @input=${(event: Event) => { this.search = (event.target as HTMLInputElement).value }} placeholder="Search references"></label>
      <p class="muted">${shown.length} of ${references.length} references</p>
      <div class="table-wrap"><table aria-label="References"><thead><tr><th scope="col">Reference</th><th scope="col">Project requirement</th><th scope="col">Source</th></tr></thead>
        <tbody>${shown.map(source => html`<tr><td>${source.title}</td><td><code>${source.requirement}</code></td><td><code>${source.owner}</code></td></tr>`)}</tbody>
      </table></div>
      ${shown.length === 0 ? html`<p role="status">No references match your search.</p>` : nothing}
      <h2>Reference guidance</h2>
      <div class="table-wrap"><table aria-label="Reference guidance"><thead><tr><th scope="col">Reference</th><th scope="col">Topics</th></tr></thead>
        <tbody>${guidance.map(item => html`<tr><td>${item.title}</td><td>${item.topics.join('; ')}</td></tr>`)}</tbody>
      </table></div>
    </article>`
  }
}

customElements.define('playground-reference-refresh', PlaygroundReferenceRefresh)
