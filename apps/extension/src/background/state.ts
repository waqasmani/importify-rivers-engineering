// Transient per-tab state kept in chrome.storage.session so it survives service-worker restarts:
// picker sessions, picked selections and the context of the last "Capture with REA" click.
import type { PickField, Selection } from '../shared/types'
import { session, sessionGet, sessionRemove, sessionSet } from './store'

export interface TabSelections {
  url: string
  items: Selection[]
}

export interface PickerSession {
  nonce: string
  field: PickField
}

export interface PendingContext {
  tabId: number
  kind: 'page' | 'selection' | 'image'
  pageUrl: string
  at: number
}

const selKey = (tabId: number) => `sel:${tabId}`
const pickerKey = (tabId: number) => `picker:${tabId}`

export async function getSelections(tabId: number): Promise<TabSelections> {
  return (await sessionGet<TabSelections>(selKey(tabId))) ?? { url: '', items: [] }
}

export async function addSelection(tabId: number, url: string, sel: Selection): Promise<number> {
  const cur = await getSelections(tabId)
  const sameDoc = !cur.url || !url || stripHash(cur.url) === stripHash(url)
  const items = sameDoc ? cur.items : []
  // One selection per assigned field, except notes, images and free text which accumulate.
  const single = sel.label === 'title' || sel.label === 'price' || sel.label === 'description' || sel.label === 'specifications'
  const next = (single ? items.filter((s) => s.label !== sel.label) : items).concat(sel).slice(-50)
  await sessionSet(selKey(tabId), { url: url || cur.url, items: next })
  return next.length
}

export async function removeSelection(tabId: number, index: number): Promise<TabSelections> {
  const cur = await getSelections(tabId)
  cur.items.splice(index, 1)
  await sessionSet(selKey(tabId), cur)
  return cur
}

export async function clearSelections(tabId: number): Promise<void> {
  await sessionRemove(selKey(tabId))
}

/** Drops selections that were made on a different page than the one being captured. */
export async function selectionsForUrl(tabId: number, url: string): Promise<TabSelections> {
  const cur = await getSelections(tabId)
  if (cur.url && url && stripHash(cur.url) !== stripHash(url)) {
    await clearSelections(tabId)
    return { url, items: [] }
  }
  return cur
}

function stripHash(u: string): string {
  return u.split('#')[0] ?? u
}

export async function getPicker(tabId: number): Promise<PickerSession | null> {
  return sessionGet<PickerSession>(pickerKey(tabId))
}

export async function setPicker(tabId: number, s: PickerSession): Promise<void> {
  await sessionSet(pickerKey(tabId), s)
}

export async function clearPicker(tabId: number): Promise<void> {
  await sessionRemove(pickerKey(tabId))
}

export async function setContext(c: PendingContext): Promise<void> {
  await sessionSet('context', c)
}

export async function takeContext(): Promise<PendingContext | null> {
  const c = await sessionGet<PendingContext>('context')
  if (c) await sessionRemove('context')
  return c && Date.now() - c.at < 10 * 60_000 ? c : null
}

export async function forgetTab(tabId: number): Promise<void> {
  await sessionRemove([selKey(tabId), pickerKey(tabId)])
}

export async function clearAllTransient(): Promise<void> {
  await session.clear()
}
