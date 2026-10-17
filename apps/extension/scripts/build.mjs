// Builds the extension into dist/: Vue pages (side panel and popup), the ES-module service
// worker, the classic content script, icons and manifest.json, then verifies the result.
import { readFile, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { build } from 'vite'
import { checkDist, EXPECTED_ID } from './check-dist.mjs'
import { writeIcons } from './icons.mjs'
import { createManifest } from './manifest.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const dist = path.join(root, 'dist')
const pkg = JSON.parse(await readFile(path.join(root, 'package.json'), 'utf8'))
const key = (await readFile(path.join(root, 'manifest-key.txt'), 'utf8')).trim()

const common = {
  configFile: false,
  envDir: false,
  logLevel: 'warn',
  publicDir: false,
  define: { 'process.env.NODE_ENV': JSON.stringify('production') },
}
const output = { target: 'chrome116', sourcemap: false, minify: true, emptyOutDir: false, outDir: dist, reportCompressedSize: false }

await rm(dist, { recursive: true, force: true })

// Side panel and popup share one Vue application.
await build({
  ...common,
  root: path.join(root, 'src/ui'),
  base: './',
  plugins: [vue(), tailwindcss()],
  build: {
    ...output,
    modulePreload: { polyfill: false },
    chunkSizeWarningLimit: 1500,
    rolldownOptions: {
      input: { sidepanel: path.join(root, 'src/ui/sidepanel.html'), popup: path.join(root, 'src/ui/popup.html') },
    },
  },
})

// Service worker: one ES module.
await build({
  ...common,
  root,
  build: {
    ...output,
    lib: { entry: path.join(root, 'src/background/index.ts'), formats: ['es'], fileName: () => 'background.js' },
  },
})

// Content script: a classic IIFE, because chrome.scripting.executeScript cannot inject modules.
await build({
  ...common,
  root,
  build: {
    ...output,
    lib: { entry: path.join(root, 'src/content/index.ts'), formats: ['iife'], name: 'REAContentScript', fileName: () => 'content.js' },
  },
})

await writeIcons(path.join(dist, 'icons'))
await writeFile(path.join(dist, 'manifest.json'), JSON.stringify(createManifest({ version: pkg.version, key }), null, 2) + '\n')

const { problems, files } = await checkDist(dist, { keyFile: path.join(root, 'manifest-key.txt') })
if (problems.length) {
  console.error('dist check failed:\n  - ' + problems.join('\n  - '))
  process.exit(1)
}
console.log(`Built REA Importify ${pkg.version} into dist/ (${files.length} files, extension ID ${EXPECTED_ID}); dist check passed.`)
