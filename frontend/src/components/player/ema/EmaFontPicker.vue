<script setup lang="ts">
/**
 * The Ema designer's font chooser: a search box over two groups of families - the
 * system faces every visitor already has, then the Google families. Each family
 * is a button rather than a native <select> option, so the list narrows as you
 * type and every entry is still a full-size tap target.
 *
 * Only the system faces are drawn in their own face. Previewing the Google ones
 * would mean downloading every family on the list; a family is fetched when it is
 * picked (see EmaDesigner.vue).
 */
import { computed, ref } from 'vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import { type EmaFontChoices, filterFonts } from '@/lib/emaDesigner'

const props = defineProps<{ choices: EmaFontChoices; label: string }>()
const family = defineModel<string>({ required: true })

const query = ref('')

const groups = computed(() =>
  [
    { name: 'Basic', families: filterFonts(props.choices.basic, query.value), preview: true },
    {
      name: 'Google Fonts',
      families: filterFonts(props.choices.google, query.value),
      preview: false,
    },
  ].filter((g) => g.families.length),
)
</script>

<template>
  <div class="ema-font-picker" role="group" :aria-label="label">
    <SearchInput v-model="query" placeholder="Search fonts" aria-label="Search fonts" />
    <div class="ema-font-list">
      <template v-for="group in groups" :key="group.name">
        <p class="ema-font-group">{{ group.name }}</p>
        <ul class="ema-font-options">
          <li v-for="name in group.families" :key="name">
            <button
              type="button"
              class="ema-font-option"
              :class="{ 'is-selected': name === family }"
              :aria-pressed="name === family"
              :style="group.preview ? { fontFamily: `'${name}'` } : undefined"
              @click="family = name"
            >
              {{ name }}
            </button>
          </li>
        </ul>
      </template>
      <p v-if="!groups.length" class="ema-font-empty">No fonts match "{{ query.trim() }}".</p>
    </div>
  </div>
</template>
