<script setup lang="ts">
/**
 * Client-side theme picker. Lets a player choose any Public theme for their own
 * browser (persisted via the app store's theme preference). "Default" always
 * follows whatever theme the admin has activated — its real name is deliberately
 * never shown. The picker is hidden entirely when there are no public themes to
 * choose from (a lone "Default" option is pointless).
 */
import { onMounted, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { useAppStore } from '@/stores/app'
import type { PublicStyle } from '@/types/api'

const app = useAppStore()
const publicThemes = ref<PublicStyle[]>([])

onMounted(async () => {
  try {
    const data = await endpoints.styles.listPublic()
    publicThemes.value = data.styles
  } catch {
    /* silent — the picker just stays hidden if the list can't load */
  }
})

function onChange(e: Event): void {
  void app.setThemePreference((e.target as HTMLSelectElement).value)
}
</script>

<template>
  <label v-if="publicThemes.length" class="theme-picker">
    <font-awesome-icon :icon="['fad', 'palette']" />
    <span class="theme-picker-label">Theme</span>
    <select :value="app.themePreference" aria-label="Choose a theme" @change="onChange">
      <option value="default">Default</option>
      <option v-for="t in publicThemes" :key="t.id" :value="String(t.id)">{{ t.name }}</option>
    </select>
  </label>
</template>

<style scoped>
.theme-picker {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 0.9rem;
}
.theme-picker-label {
  font-weight: 600;
}
.theme-picker select {
  width: auto;
}
</style>
