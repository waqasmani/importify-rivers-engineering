import { defineConfig } from 'vitest/config'

// Pages are parsed as inert data: happy-dom never runs their scripts or loads their resources.
export default defineConfig({
  test: {
    environment: 'happy-dom',
    environmentOptions: {
      happyDOM: {
        settings: {
          disableJavaScriptEvaluation: true,
          disableJavaScriptFileLoading: true,
          disableCSSFileLoading: true,
          disableIframePageLoading: true,
          handleDisabledFileLoadingAsSuccess: true,
        },
      },
    },
    include: ['tests/**/*.spec.ts'],
    globals: false,
  },
})
