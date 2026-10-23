/**
 * Type definitions for Datastar Inspector
 */

/**
 * Per-tab inspector state (stored in sessionStorage)
 */
export interface InspectorState {
  /** Whether the panel is expanded */
  expanded: boolean
  /** Current filter text */
  filter: string
  /** Expanded tree paths */
  expandedPaths?: string[]
}

/**
 * Per-browser inspector preferences (stored in localStorage) so a hidden or
 * moved inspector stays out of the way across tabs and pages.
 */
export interface InspectorPreferences {
  /** Whether the inspector is hidden entirely */
  hidden?: boolean
  /** User-positioned collapsed launcher coordinates */
  togglePosition?: InspectorPosition
  /** User-positioned expanded panel coordinates */
  panelPosition?: InspectorPosition
}

export interface InspectorPosition {
  x: number
  y: number
}

/**
 * Signal data structure (recursive key-value)
 */
export type SignalValue = string | number | boolean | null | SignalValue[] | SignalObject

export interface SignalObject {
  [key: string]: SignalValue
}
