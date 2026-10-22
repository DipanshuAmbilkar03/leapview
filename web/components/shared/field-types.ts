/**
 * Whether a warehouse or semantic type name is numeric. One definition serves
 * the preview table (alignment), the Data Explorer (filter operators and input
 * modes) and the field type icons; they previously carried three diverging
 * copies, and the filter copy missed `real`.
 */
export function isNumericType(type: string | undefined): boolean {
  return /int|decimal|double|float|number|numeric|real/i.test(type ?? '')
}
