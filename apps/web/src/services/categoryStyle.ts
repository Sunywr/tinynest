/** Category colour palette shared by the entry grid, the bills list, the chart
 *  ranking and the category editor. Keys are stored in `categories.color`, so a
 *  colour choice survives a reload and every surface renders it identically. */

export const CATEGORY_COLOR_KEYS = ['1', '2', '3', '4', '5', '6', '7', '8'] as const
export type CategoryColorKey = (typeof CATEGORY_COLOR_KEYS)[number]

export const CATEGORY_COLOR_LABELS: Record<CategoryColorKey, string> = {
  '1': '珊瑚',
  '2': '杏黄',
  '3': '橄榄',
  '4': '青雾',
  '5': '紫藤',
  '6': '玫瑰',
  '7': '陶土',
  '8': '苔绿',
}

export function isCategoryColorKey(value: unknown): value is CategoryColorKey {
  return typeof value === 'string' && (CATEGORY_COLOR_KEYS as readonly string[]).includes(value)
}

/** Resolves a stored colour to the palette key, falling back to a stable colour
 *  derived from the category's position so legacy rows still look intentional. */
export function categoryColorKey(color: string | null | undefined, index = 0): CategoryColorKey {
  if (isCategoryColorKey(color)) return color
  const position = Math.abs(Math.trunc(index)) % CATEGORY_COLOR_KEYS.length
  return CATEGORY_COLOR_KEYS[position]
}

export function categoryBackground(color: string | null | undefined, index = 0): string {
  return `var(--cat-${categoryColorKey(color, index)})`
}

export function categoryInk(color: string | null | undefined, index = 0): string {
  return `var(--cat-${categoryColorKey(color, index)}-ink)`
}
