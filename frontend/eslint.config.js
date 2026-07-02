import shared from '@jwill89/eslint-config'

// Flat ESLint config for the Vue 3 + TypeScript SPA. The rules, plugin versions,
// and formatting come from the shared @jwill89/eslint-config — a git submodule at
// ./eslint-config, installed as a local (`file:`) dev dependency — so every
// jwill89 frontend lints against one source of truth. Only project-specific
// ignores live here (the generated tygo types + the submodule dir itself).
export default [
  {
    ignores: [
      'dist/**',
      'dist-ssr/**',
      'coverage/**',
      'eslint-config/**',
      'src/types/api.generated.ts',
      // Root JS config files aren't part of any tsconfig, so the shared config's
      // type-aware (projectService) rules can't resolve them — and they don't need
      // linting. (vite/vitest .ts configs stay linted via tsconfig.node.json.)
      'eslint.config.js',
      'prettier.config.js',
    ],
  },

  ...shared,
]
