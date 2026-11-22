/** Front-end xlsx export of the current bill filter set. No new API endpoint:
 *  the existing cursor-paginated /transactions list is read and rendered locally.
 *  Amounts are converted to yuan only for the spreadsheet cell text. */

import * as XLSX from 'xlsx'
import { api, type Transaction, type TransactionQuery } from './api'

const PAGE_LIMIT = 200
const MAX_PAGES = 50 // hard cap: 10,000 rows, avoids an unbounded export loop

export type ExportFilter = Pick<TransactionQuery, 'from' | 'to' | 'type' | 'q'>

const TYPE_LABEL: Record<string, string> = { expense: '支出', income: '收入', transfer: '转账' }

function yuan(minor: number): string {
  return (minor / 100).toFixed(2)
}

export function exportRows(items: Transaction[]): (string | number)[][] {
  const header = ['日期', '类型', '分类', '备注', '账户', '转入账户', '成员', '金额（元）', '状态']
  const rows = items.map(t => [
    t.occurred_on,
    TYPE_LABEL[t.type] ?? t.type,
    t.category_name || '',
    t.note || '',
    t.account_name || '',
    t.to_account_name || '',
    t.member_id || '',
    yuan(t.amount_minor),
    t.status === 'reversed' ? '已冲正' : '已入账',
  ])
  return [header, ...rows]
}

async function collect(filter: ExportFilter): Promise<Transaction[]> {
  const items: Transaction[] = []
  let cursor: string | undefined
  for (let page = 0; page < MAX_PAGES; page += 1) {
    const result = await api.transactions({
      ...filter,
      limit: PAGE_LIMIT,
      cursor,
    })
    items.push(...result.items)
    if (!result.next_cursor) break
    cursor = result.next_cursor
  }
  return items
}

function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Export the filter set as .xlsx and resolve with the exported row count. */
export async function exportBills(filter: ExportFilter): Promise<number> {
  const items = await collect(filter)
  if (!items.length) return 0
  const sheet = XLSX.utils.aoa_to_sheet(exportRows(items))
  sheet['!cols'] = [
    { wch: 12 }, { wch: 8 }, { wch: 12 }, { wch: 24 },
    { wch: 14 }, { wch: 14 }, { wch: 10 }, { wch: 12 }, { wch: 8 },
  ]
  const book = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(book, sheet, '账单')
  const bytes = XLSX.write(book, { bookType: 'xlsx', type: 'array' }) as ArrayBuffer
  const name = `tinynest-账单-${filter.from || '全部'}-${filter.to || '全部'}.xlsx`
  download(new Blob([bytes], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), name)
  return items.length
}
