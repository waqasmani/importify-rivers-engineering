// Extension-origin storage. IndexedDB (service worker origin) holds the refresh token, device
// identity, server address and preferences; chrome.storage.session holds the short-lived access
// token and transient state with access restricted to trusted extension contexts. Nothing is
// kept in chrome.storage.local, which content scripts could read.
import type { CredentialStore, StoredAccess, StoredCredentials } from './auth'

const DB_NAME = 'rea-extension'
const STORE = 'kv'

function promisify<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error ?? new Error('IndexedDB request failed'))
  })
}

export class IdbKv {
  private db: Promise<IDBDatabase> | null = null

  private open(): Promise<IDBDatabase> {
    if (!this.db) {
      this.db = new Promise((resolve, reject) => {
        const req = indexedDB.open(DB_NAME, 1)
        req.onupgradeneeded = () => {
          if (!req.result.objectStoreNames.contains(STORE)) req.result.createObjectStore(STORE)
        }
        req.onsuccess = () => resolve(req.result)
        req.onerror = () => reject(req.error ?? new Error('IndexedDB could not be opened'))
      })
    }
    return this.db
  }

  async get<T>(key: string): Promise<T | null> {
    const db = await this.open()
    const v = await promisify(db.transaction(STORE, 'readonly').objectStore(STORE).get(key))
    return (v as T | undefined) ?? null
  }

  async set(key: string, value: unknown): Promise<void> {
    const db = await this.open()
    await promisify(db.transaction(STORE, 'readwrite').objectStore(STORE).put(value, key))
  }

  async delete(key: string): Promise<void> {
    const db = await this.open()
    await promisify(db.transaction(STORE, 'readwrite').objectStore(STORE).delete(key))
  }

  async clear(): Promise<void> {
    const db = await this.open()
    await promisify(db.transaction(STORE, 'readwrite').objectStore(STORE).clear())
  }
}

export const kv = new IdbKv()
export const session = chrome.storage.session

export async function sessionGet<T>(key: string): Promise<T | null> {
  const v = await session.get(key)
  return (v[key] as T | undefined) ?? null
}

export async function sessionSet(key: string, value: unknown): Promise<void> {
  await session.set({ [key]: value })
}

export async function sessionRemove(keys: string | string[]): Promise<void> {
  await session.remove(keys)
}

export const credentialStore: CredentialStore = {
  load: () => kv.get<StoredCredentials>('credentials'),
  save: (c) => kv.set('credentials', c),
  loadAccess: () => sessionGet<StoredAccess>('access'),
  saveAccess: (a) => sessionSet('access', a),
  async wipe() {
    await kv.delete('credentials')
    await sessionRemove(['access', 'config'])
  },
}
