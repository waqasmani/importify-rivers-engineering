// @rea/extractor: reads product data from a DOM Document. It uses only DOM APIs, so the same code
// runs in the extension's content script and in the dashboard (on documents created with
// DOMParser, which never run scripts or load resources).
export * from './types'
export * from './adapter'
export * from './dom'
export * from './price'
export * from './sensitive'
export { mergeResults, type AdapterResult } from './merge'
export { detectPageType, inspectPage } from './inspect'
export { analyzePage, applicableAdapters, domainMatches, pageContext, runAdapter, runnable, BUILTIN, type AnalyzeOptions } from './page'
export { adaptersFromApi, toAdapterConfig, MAX_DOMAIN_PATTERNS, type ApiAdapter } from './config'
export { runRulesAdapter, describeSource } from './rules/engine'
export {
  parseRuleDocument, ruleDocumentSchema, ruleFieldsOf, RULE_FIELDS, RULES_SCHEMA_VERSION, MAX_RULES_BYTES,
  type RuleDocument, type RuleField, type RuleIssue, type RuleCandidate, type RuleSource, type RuleVariants, type RuleCondition,
} from './rules/schema'
export { evalPath, parsePath, pathError, PATH_LIMITS, PathError } from './rules/path'
export { scanBalanced, decodeJsString, parseLoose, parseData, valueAfterMarker, JsonScanError, MAX_SCRIPT_BYTES, MAX_DEPTH } from './rules/json'
export { applyTransform, applyTransforms, originalImage, SIMPLE_TRANSFORMS, type Transform } from './rules/transforms'
export { cleanDocTitle } from './adapters/generic'
