<script setup lang="ts">
/**
 * The toolbar above a {@link DataTable}. One layout for every table, because
 * seven tables had each grown their own: the row count sat left of the actions
 * on one screen and right of them on the next, the same button read "CSV" here
 * and "Export CSV" there, the counts said "6/6 cards", "40 total entries",
 * "10 stamps" and "3 draws", and Manage Cards split its controls over two rows
 * while the rest used one. Composing the pieces by hand is what let that happen,
 * so the order is fixed here rather than left to each caller:
 *
 *   [ search  filters ] ................... [ per-page  count  actions ]
 *
 * Inputs read first on the left; state and the things that act on it collect on
 * the right, with the count immediately before the buttons so it reads as
 * "40 entries -> [Export] [Delete]".
 *
 * Slots: `search` (a `SearchInput`), `filters` (facet selects), `actions`
 * (buttons - use `btn-sm`, so they match the row actions in the table below).
 */
import { computed } from 'vue'

const props = defineProps<{
  /** Rows matching the current filters - what the table is showing. */
  count: number
  /** Unfiltered row count. Given and different, the count reads "N of M". */
  total?: number
  /** Singular noun for the rows ("card", "entry", "stamp", "draw"). */
  noun: string
  /** Plural, when it isn't `noun + "s"` ("entries", "matches"). */
  plural?: string
  /** Page-size options. Omit to leave the per-page control out. */
  perPageOptions?: number[]
}>()

/** Page size, when `perPageOptions` is given. */
const perPage = defineModel<number>('perPage')

const label = computed(() => (props.count === 1 ? props.noun : (props.plural ?? `${props.noun}s`)))

/** "40 entries", or "12 of 40 entries" once a filter is narrowing the set. */
const countText = computed(() =>
  props.total != null && props.total !== props.count
    ? `${props.count} of ${props.total} ${label.value}`
    : `${props.count} ${label.value}`,
)
</script>

<template>
  <div class="dt-toolbar">
    <div class="dt-toolbar-start">
      <slot name="search" />
      <slot name="filters" />
    </div>
    <div class="dt-toolbar-end">
      <label v-if="perPageOptions" class="dt-toolbar-perpage">
        <span class="text-muted text-xs">Per page:</span>
        <select v-model.number="perPage" aria-label="Rows per page">
          <option v-for="n in perPageOptions" :key="n" :value="n">{{ n }}</option>
        </select>
      </label>
      <span class="dt-toolbar-count">{{ countText }}</span>
      <slot name="actions" />
    </div>
  </div>
</template>
