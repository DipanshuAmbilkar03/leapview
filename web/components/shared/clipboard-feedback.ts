import { copyText, type CopyOptions } from './clipboard'
import { showToast } from './toast'

export type CopyFeedback = CopyOptions & {
  /** Names what was copied in success messages, for example "Temporary password". */
  label: string
  /** Names it mid-sentence in failure messages; defaults to the lower-cased label. */
  noun?: string
}

/** Copies text and reports the outcome in the global toast region; resolves true when copied. */
export async function copyTextWithFeedback(value: string, { label, noun = label.toLowerCase(), selectOnFailure }: CopyFeedback): Promise<boolean> {
  const copied = await copyText(value, { selectOnFailure })
  if (copied) {
    showToast({ message: `${label} copied`, tone: 'success' })
  } else {
    showToast({
      message: selectOnFailure ? `Could not copy the ${noun}. It is selected, so press Ctrl+C or Cmd+C.` : `Could not copy the ${noun}. Select it and copy it manually.`,
      tone: 'error',
      durationMs: 0,
    })
  }
  return copied
}
