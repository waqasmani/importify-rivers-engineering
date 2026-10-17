// Extension types. Extraction types come from @rea/extractor, which the content script and the
// dashboard share; this file adds what only the extension needs (selections, picker, snapshots,
// the content-script API).
import type { AdapterConfig, PageExtraction, Schemas } from '@rea/extractor'

export {
  PAGE_TYPES, FIELD_KEYS, METADATA_KEYS, BUILTIN_ADAPTERS,
  type Schemas, type Payload, type PayloadImage, type PayloadVariant, type PageType, type FieldKey, type MetadataKey, type FieldInfo,
  type MetadataValue, type AdapterRun, type ProductExtraction, type MicrodataItem, type InspectorData, type AdapterConfig, type AdapterKind,
  type PageExtraction,
} from '@rea/extractor'

/** A user selection (API Selection with the kinds the server accepts). */
export interface Selection {
  kind: 'text' | 'table' | 'element' | 'image' | 'link'
  text: string
  selector: string
  rows?: string[][]
  src?: string
  alt?: string
  label?: string
}
export type CaptureInput = Schemas['CaptureInput']
export type ImportRequest = Schemas['ImportRequest']
export type ExtensionConfig = Schemas['ExtensionConfigDoc']

/** Fields the element picker can assign. */
export const PICK_FIELDS = ['title', 'price', 'image', 'description', 'specifications', 'note'] as const
export type PickField = (typeof PICK_FIELDS)[number]
export const PICK_FIELD_LABELS: Record<PickField, string> = {
  title: 'Title',
  price: 'Price',
  image: 'Image',
  description: 'Description',
  specifications: 'Specifications table',
  note: 'Custom note',
}

export interface SnapshotInfo {
  included: boolean
  bytes: number
  html: string | null
  note: string
}

export interface ExtractionResult extends PageExtraction {
  snapshot: SnapshotInfo
  extractedAt: string
}

export interface ExtractOptions {
  adapters: AdapterConfig[]
  snapshot: { enabled: boolean; maxBytes: number }
}

export interface PickerOptions {
  nonce: string
  field: PickField
}

/** The API exposed by the injected content script inside the extension's isolated world. */
export interface ContentApi {
  version: number
  extract(options: unknown): ExtractionResult
  startPicker(options: unknown): { started: boolean }
  stopPicker(): { stopped: boolean }
}

declare global {
  // eslint-disable-next-line no-var
  var __reaContent: ContentApi | undefined
}
