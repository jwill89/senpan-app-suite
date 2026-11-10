/**
 * Game presets store: CRUD for reusable game templates (a named set of win
 * pattern IDs + pre-written markdown game details). Presets are selected on the
 * Game tab when starting a new game to auto-apply their patterns and details.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, unwrap, type GamePreset, type GamePresetInput } from '@/api/client'
import { DEFAULT_AUTO_INTERVAL } from '@/lib/constants'
import { useUiStore } from './ui'

/** The editable form model for a preset (id 0 = new). */
export interface PresetForm extends GamePresetInput {
  id: number
}

export const usePresetsStore = defineStore('presets', () => {
  const ui = useUiStore()

  const presets = ref<GamePreset[]>([])
  /** True while the preset list is loading (drives the list spinner). */
  const presetsLoading = ref(false)
  /** True while the current preset is being saved (drives the Save button). */
  const savingPreset = ref(false)
  /** The preset currently open in the editor, or null when the form is closed. */
  const editingPreset = ref<PresetForm | null>(null)

  async function loadPresets(): Promise<void> {
    presetsLoading.value = true
    try {
      presets.value = (await unwrap(api.GET('/presets'))).presets
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      presetsLoading.value = false
    }
  }

  /** Open a blank editor for a new preset. */
  function newPreset(): void {
    editingPreset.value = {
      id: 0,
      name: '',
      patternIds: [],
      gameDetails: '',
      auto: false,
      autoInterval: DEFAULT_AUTO_INTERVAL,
    }
  }

  /** Open the editor on an existing preset (copies its values). */
  function editPreset(preset: GamePreset): void {
    editingPreset.value = {
      id: preset.id,
      name: preset.name,
      patternIds: [...preset.patternIds],
      gameDetails: preset.gameDetails || '',
      auto: preset.auto,
      autoInterval: preset.autoInterval || DEFAULT_AUTO_INTERVAL,
    }
  }

  function cancelEdit(): void {
    editingPreset.value = null
  }

  async function savePreset(): Promise<void> {
    if (!editingPreset.value) {
      return
    }
    const { id, ...form } = editingPreset.value
    const name = form.name.trim()
    if (!name) {
      ui.notify('Preset name is required', 'error')
      return
    }
    if (form.patternIds.length === 0) {
      ui.notify('Select at least one win pattern', 'error')
      return
    }
    const body: GamePresetInput = { ...form, name }
    savingPreset.value = true
    try {
      if (id) {
        await unwrap(api.PUT('/presets/{id}', { params: { path: { id } }, body }))
        ui.notify('Preset saved', 'success')
      } else {
        await unwrap(api.POST('/presets', { body }))
        ui.notify('Preset created', 'success')
      }
      editingPreset.value = null
      await loadPresets()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      savingPreset.value = false
    }
  }

  async function deletePreset(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this preset?', { title: 'Delete preset', confirmText: 'Delete' }))
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/presets/{id}', { params: { path: { id } } }))
      if (editingPreset.value?.id === id) {
        editingPreset.value = null
      }
      ui.notify('Preset deleted', 'info')
      await loadPresets()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  return {
    presets,
    presetsLoading,
    savingPreset,
    editingPreset,
    loadPresets,
    newPreset,
    editPreset,
    cancelEdit,
    savePreset,
    deletePreset,
  }
})
