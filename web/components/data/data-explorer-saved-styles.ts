import { css } from 'lit'

export const savedExplorationStyles = css`
  .saved-explorations {
    position: relative;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--base-size-8);
    border-bottom: var(--lv-border-muted);
    padding: var(--base-size-8) var(--base-size-16);
  }

  .saved-explorations-header,
  .saved-exploration-actions {
    display: flex;
    align-items: center;
    gap: var(--base-size-8);
  }

  .saved-exploration-actions {
    display: grid;
    position: absolute;
    z-index: var(--zIndex-overlay);
    top: 100%;
    right: var(--base-size-12);
    width: min(20rem, calc(100% - 24px));
    max-height: min(24rem, 60svh);
    overflow: auto;
    box-sizing: border-box;
    padding: var(--base-size-12);
    border: var(--lv-border-default);
    border-radius: var(--lv-radius-default);
    background: var(--lv-bg-overlay);
    box-shadow: var(--lv-shadow-floating-sm);
  }

  .saved-exploration-actions[hidden] { display: none; }
  .saved-exploration-actions label { display: grid; gap: var(--base-size-4); font: var(--lv-type-caption); }
  .saved-exploration-actions .archive-action { border-top: var(--lv-border-muted); margin-top: var(--base-size-4); }
  .saved-exploration-toolbar { display: flex; gap: var(--base-size-4); margin-left: auto; flex: none; }
  .saved-actions-toggle { display: inline-flex; align-items: center; gap: var(--base-size-4); }

  .saved-exploration-actions input,
  .saved-exploration-actions select {
    min-height: 2rem;
    border: var(--lv-border-default);
    border-radius: var(--lv-radius-default);
    padding: 0 var(--base-size-8);
    background: var(--lv-bg-control);
    color: var(--lv-fg-default);
    font: var(--lv-type-body);
  }

  .saved-exploration-actions input {
    box-sizing: border-box;
    width: 100%;
    min-width: 0;
  }

  .saved-exploration-actions .text-button {
    white-space: nowrap;
    justify-content: flex-start;
    text-align: left;
  }

  .saved-explorations-header {
    flex: 1;
    flex-wrap: wrap;
    min-width: 0;
  }

  .saved-explorations-title {
    color: var(--lv-fg-muted);
    font: var(--lv-type-caption);
    font-weight: var(--base-text-weight-medium);
  }

  .saved-exploration-picker {
    position: relative;
    min-width: 0;
  }

  .saved-exploration-picker summary {
    display: inline-flex;
    min-height: var(--control-small-size);
    align-items: center;
    gap: var(--base-size-4);
    color: var(--lv-fg-muted);
    cursor: pointer;
    font: var(--lv-type-caption);
    font-weight: var(--base-text-weight-medium);
    list-style: none;
    max-width: 100%;
    text-transform: none;
  }
  .saved-exploration-picker summary span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .saved-exploration-picker summary svg { flex: none; }

  .saved-exploration-picker summary::-webkit-details-marker {
    display: none;
  }

  .saved-exploration-picker summary:hover,
  .saved-exploration-picker summary:focus-visible {
    color: var(--lv-fg-default);
  }

  .saved-exploration-picker summary:focus-visible {
    outline: var(--lv-border-width-focus) solid var(--lv-line-accent);
    outline-offset: var(--base-size-2);
  }

  .saved-exploration-list {
    position: absolute;
    top: calc(100% + var(--base-size-4));
    left: 0;
    z-index: var(--zIndex-overlay);
    display: grid;
    width: min(18rem, calc(100vw - 2rem));
    max-height: min(20rem, calc(100vh - 6rem));
    gap: var(--base-size-2);
    overflow: auto;
    border: var(--lv-border-default);
    border-radius: var(--lv-radius-default);
    background: var(--lv-bg-overlay);
    box-shadow: var(--lv-shadow-floating-sm);
    padding: var(--base-size-4);
  }

  .saved-exploration-item {
    overflow: hidden;
    min-height: var(--control-small-size);
    border-radius: var(--lv-radius-default);
    padding: var(--base-size-6) var(--base-size-8);
    color: var(--lv-fg-default);
    text-decoration: none;
    text-overflow: ellipsis;
    white-space: nowrap;
    font: var(--lv-type-caption);
  }

  .saved-exploration-item:hover,
  .saved-exploration-item:focus-visible {
    background: var(--lv-bg-control-hover);
    outline: 0;
  }

  .saved-exploration-status {
    min-width: 0;
    overflow-wrap: anywhere;
    color: var(--lv-fg-muted);
    font: var(--lv-type-caption);
  }

  .header { position: relative; }
  .saved-exploration-sharing {
    position: static;
    min-width: 0;
    color: var(--lv-fg-muted);
    font: var(--lv-type-caption);
  }

  .saved-exploration-sharing > summary {
    display: inline-flex;
    min-height: var(--control-medium-size);
    align-items: center;
    gap: var(--base-size-6);
    border: var(--lv-border-default);
    border-radius: var(--lv-radius-default);
    background: var(--lv-bg-control);
    color: var(--lv-fg-default);
    cursor: pointer;
    list-style: none;
    padding: 0 var(--base-size-12);
    font: var(--lv-type-body);
    font-weight: var(--base-text-weight-medium);
    text-transform: none;
    white-space: nowrap;
  }

  .saved-exploration-sharing summary::-webkit-details-marker {
    display: none;
  }

  .saved-exploration-sharing > summary:hover,
  .saved-exploration-sharing[open] > summary {
    background: var(--lv-bg-control-hover);
  }

  .saved-exploration-sharing summary:focus-visible {
    outline: var(--lv-border-width-focus) solid var(--lv-line-accent);
    outline-offset: var(--base-size-2);
  }

  .saved-exploration-sharing-actions {
    box-sizing: border-box;
    position: absolute;
    top: calc(100% + var(--base-size-4));
    right: var(--base-size-12);
    z-index: var(--zIndex-overlay);
    display: grid;
    width: min(20rem, calc(100% - 24px));
    max-height: min(32rem, 65svh);
    gap: var(--base-size-4);
    overflow: auto;
    border: var(--lv-border-default);
    border-radius: var(--lv-radius-default);
    background: var(--lv-bg-overlay);
    box-shadow: var(--lv-shadow-floating-sm);
    padding: var(--base-size-8);
  }

  .saved-exploration-sharing-label {
    padding: var(--base-size-4) var(--base-size-8);
    color: var(--lv-fg-muted);
    font: var(--lv-type-caption);
    font-weight: var(--base-text-weight-medium);
    text-transform: uppercase;
  }

  .saved-exploration-sharing-label.export-label {
    border-top: var(--lv-border-muted);
    margin-top: var(--base-size-4);
    padding-top: var(--base-size-12);
  }

  .saved-exploration-sharing-actions .text-button,
  .saved-exploration-download,
  .saved-exploration-share-fallback {
    display: flex;
    box-sizing: border-box;
    width: 100%;
    align-items: center;
    gap: var(--base-size-8);
    min-height: var(--control-medium-size);
    border: 0;
    border-radius: var(--lv-radius-default);
    background: transparent;
    padding: var(--base-size-6) var(--base-size-8);
    color: var(--lv-fg-default);
    font: var(--lv-type-body);
    font-weight: var(--base-text-weight-medium);
    text-decoration: none;
    text-align: left;
    cursor: pointer;
  }

  .saved-exploration-sharing-actions svg { flex: none; color: var(--lv-fg-muted); }
  .saved-exploration-sharing-actions .dashboard-append-picker > summary { font: var(--lv-type-body); text-transform: none; }
  .saved-exploration-sharing-actions .dashboard-append-chevron { margin-left: auto; }
  .saved-exploration-sharing-actions .dashboard-append-picker[open] .dashboard-append-chevron { transform: rotate(180deg); }
  .saved-exploration-sharing-actions [role='status'] { padding: var(--base-size-4) var(--base-size-8); }

  .saved-exploration-sharing-actions .text-button:hover,
  .saved-exploration-sharing-actions .text-button:focus-visible,
  .saved-exploration-download:hover,
  .saved-exploration-download:focus-visible,
  .saved-exploration-share-fallback:hover,
  .saved-exploration-share-fallback:focus-visible {
    background: var(--lv-bg-control-hover);
  }

  .saved-exploration-sharing-hint {
    border-top: var(--lv-border-muted);
    margin: var(--base-size-4) 0 0;
    padding: var(--base-size-8) var(--base-size-8) var(--base-size-4);
    line-height: 1.5;
  }

  .saved-exploration-export-unavailable {
    padding: var(--base-size-6) var(--base-size-8);
    color: var(--lv-fg-muted);
    font: var(--lv-type-caption);
  }

`
