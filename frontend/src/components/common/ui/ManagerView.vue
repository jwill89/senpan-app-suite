<script setup lang="ts">
/**
 * Standard "manage items" page shell — the shared structure behind every admin
 * tab that lists/manages a collection (announcements, raffles, patterns, …).
 *
 * Renders the `.admin-panel` card with a header (icon + title on the left, the
 * `#actions` slot on the right for buttons like "Manage Categories" / "+ New"),
 * an optional `#toolbar` slot (search + filters), the list/table in the default
 * slot, and an optional `#pagination` slot.
 *
 * Pair with `ListRow` (list items), `SearchInput` (toolbar), `SubPageHeader`
 * (the sub-screens the header actions open), and DataTable/PaginationBar/
 * EmptyState as needed. Convention: the tab component holds a `screen` ref and
 * the `#actions` buttons switch it; each sub-screen opens with `SubPageHeader`.
 */
import type { IconPrefix } from '@fortawesome/fontawesome-svg-core'

defineProps<{
  title?: string
  /** FontAwesome icon for the heading as `[prefix, name]`, e.g. `['fad', 'megaphone']`. */
  icon?: [IconPrefix, string]
}>()
</script>

<template>
  <div class="admin-panel">
    <div class="manager-header">
      <h3 v-if="title || $slots.title">
        <slot name="title"><font-awesome-icon v-if="icon" :icon="icon" />{{ title }}</slot>
      </h3>
      <div v-if="$slots.actions" class="manager-actions"><slot name="actions" /></div>
    </div>
    <div v-if="$slots.toolbar" class="manager-toolbar"><slot name="toolbar" /></div>
    <slot />
    <slot name="pagination" />
  </div>
</template>
