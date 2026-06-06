import { globalIgnores } from 'eslint/config'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'
import pluginVue from 'eslint-plugin-vue'
import skipFormatting from '@vue/eslint-config-prettier/skip-formatting'

// Flat ESLint config for the Vue 3 + TypeScript SPA. Formatting is delegated to
// Prettier (skipFormatting disables stylistic ESLint rules that would conflict).
export default defineConfigWithVueTs(
  {
    name: 'app/files',
    files: ['**/*.{ts,mts,tsx,vue}'],
  },

  globalIgnores(['**/dist/**', '**/dist-ssr/**', '**/coverage/**', '**/node_modules/**']),

  pluginVue.configs['flat/essential'],
  vueTsConfigs.recommended,

  {
    name: 'app/rules',
    rules: {
      // The generated API types file uses interfaces tygo emits verbatim.
      '@typescript-eslint/no-explicit-any': 'warn',
      // Allow intentional empty catch blocks (best-effort logout, JSON parse).
      'no-empty': ['error', { allowEmptyCatch: true }],
    },
  },

  skipFormatting,
)

