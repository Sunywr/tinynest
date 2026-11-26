/** Reading a bank's own XLS detail report in the browser.
 *
 *  The server decides what a line means and what may enter the ledger; this file
 *  only lifts cells out of the spreadsheet, exactly as printed, so nothing here
 *  can turn a bank report into a booking. Amounts stay as the bank's text and the
 *  server parses them, which keeps the yuan→cent conversion on one side.
 */

import * as XLSX from 'xlsx'

// A card bill is a few hundred lines. The cap mirrors the server's own limit and
// stops a wrong file (a full year of exports, a statement of someone else's) from
// being turned into a request the server would refuse anyway.
export const MAX_DETAIL_ROWS = 3000
export const MAX_DETAIL_BYTES = 8 * 1024 * 1024

export type DetailWorkbook = {
  rows: string[][]
  skipped: number
  sheetName: string
  // The card tail the report itself repeats, used only to prefill the field the
  // operator confirms — the server re-checks every row against it.
  tail: string
}

const TAIL_COLUMN = 3

export async function readDetailWorkbook(file: File): Promise<DetailWorkbook> {
  if (file.size > MAX_DETAIL_BYTES) throw new Error(`明细文件超过 ${MAX_DETAIL_BYTES / 1024 / 1024}MB，请只导出本期账单`)
  const book = XLSX.read(new Uint8Array(await file.arrayBuffer()), { type: 'array', cellDates: false })
  const sheetName = pickSheet(book.SheetNames)
  if (!sheetName) throw new Error('文件里没有工作表')
  const sheet = book.Sheets[sheetName]
  // header:1 + raw:false keeps every cell as the text the bank printed, so the
  // 8-digit dates and the decimal amounts reach the server unchanged.
  const raw = XLSX.utils.sheet_to_json<(string | number | null)[]>(sheet, { header: 1, raw: false, defval: '' })
  const rows = raw.map(row => row.map(cell => String(cell ?? '').trim()))
  if (rows.length > MAX_DETAIL_ROWS) throw new Error(`明细超过 ${MAX_DETAIL_ROWS} 行，请分账期导出`)
  // A blank spreadsheet line is not a detail row: dropping it here keeps the
  // counts the operator sees equal to the rows the server will classify.
  const data = rows.filter(row => row.some(cell => cell !== ''))
  return { rows: data, skipped: rows.length - data.length, sheetName, tail: readTail(data) }
}

function pickSheet(names: string[]): string {
  const named = names.find(name => name.includes('明细') || name.includes('账单'))
  return named ?? names[0] ?? ''
}

function readTail(rows: string[][]): string {
  const counts = new Map<string, number>()
  for (const row of rows.slice(1)) {
    const value = (row[TAIL_COLUMN] ?? '').replace(/\D/g, '')
    if (value.length >= 4) counts.set(value.slice(-4), (counts.get(value.slice(-4)) ?? 0) + 1)
  }
  let best = ''
  let most = 0
  for (const [tail, count] of counts) if (count > most) [best, most] = [tail, count]
  return best
}
