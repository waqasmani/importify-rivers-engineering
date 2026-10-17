// Types generated from packages/api-types/openapi.json. Regenerate with `pnpm api-types`.
export type { paths, components, operations } from './schema'
import type { components } from './schema'

/** All named schemas from the OpenAPI document. */
export type Schemas = components['schemas']
/** Shorthand for one named schema. */
export type Schema<K extends keyof Schemas> = Schemas[K]
