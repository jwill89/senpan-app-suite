<script setup lang="ts">
/**
 * Card preview modal (admin) — shows a card's board, a Copy-ID button, and
 * inline-editable player name + details (double-click to edit, Enter/blur to
 * save, Escape to cancel). Mirrors the original "Card preview modal" block.
 *
 * Autofocus of the edit input (which app.js did via $refs + $nextTick) is
 * reproduced with a template ref watched against the editing field.
 */
import { nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import BingoBoard from '@/components/common/BingoBoard.vue'
import { useCardsStore } from '@/stores/cards'
import { useUiStore } from '@/stores/ui'

const cards = useCardsStore()
const ui = useUiStore()
const router = useRouter()

const editInput = ref<HTMLInputElement | null>(null)

/** Copies the card's full playable URL (origin + /play/:cardId) to clipboard. */
function copyCardUrl(): void {
  if (!cards.previewCard) return
  const href = router.resolve({ name: 'player', params: { cardId: cards.previewCard.id } }).href
  ui.copyToClipboard(window.location.origin + href)
}

// Focus the inline-edit input when an edit begins (matches app.js behaviour).
watch(
  () => cards.previewCardEditing,
  async (field) => {
    if (field) {
      await nextTick()
      editInput.value?.focus()
    }
  },
)
</script>

<template>
  <ModalOverlay
    v-if="cards.previewCard"
    :box-style="{ maxWidth: '560px' }"
    @close="cards.previewCard = null"
  >
    <div class="flex-between mb-12">
      <h3 class="nowrap">
        Card <span class="code-gold">{{ cards.previewCard.id }}</span>
      </h3>
      <div class="flex-toolbar" style="flex-wrap: nowrap; gap: 8px">
        <button
          class="btn-view btn-sm nowrap"
          title="Copy card ID"
          @click="ui.copyToClipboard(cards.previewCard.id)"
        >
          <font-awesome-icon :icon="['fas', 'copy']" /> Copy ID
        </button>
        <button class="btn-view btn-sm nowrap" title="Copy playable card URL" @click="copyCardUrl">
          <font-awesome-icon :icon="['fas', 'link']" /> Copy URL
        </button>
        <button class="btn-neutral btn-sm" @click="cards.previewCard = null">Close</button>
      </div>
    </div>
    <div class="flex-center">
      <BingoBoard :board="cards.previewCard.board_data" mode="preview" preview />
    </div>
    <div class="mt-12 text-center">
      <template v-if="cards.previewCardEditing === 'player_name'">
        <input
          ref="editInput"
          v-model="cards.previewCardEditValue"
          placeholder="Player name"
          class="inline-edit-input ta-center"
          style="font-weight: 600"
          @blur="cards.savePreviewCardField('player_name')"
          @keydown.enter="($event.target as HTMLInputElement).blur()"
          @keydown.escape="cards.previewCardEditing = null"
        />
      </template>
      <p
        v-else
        style="font-weight: 600; cursor: pointer"
        :title="
          cards.previewCard.player_name ? 'Double-click to edit' : 'Double-click to set player name'
        "
        @dblclick="cards.startPreviewCardEdit('player_name')"
      >
        {{ cards.previewCard.player_name || 'No player assigned' }}
        <span v-if="!cards.previewCard.player_name" class="text-dim text-sm">
          (double-click to edit)</span
        >
      </p>
      <template v-if="cards.previewCardEditing === 'details'">
        <input
          ref="editInput"
          v-model="cards.previewCardEditValue"
          placeholder="Details (e.g. character name)"
          class="inline-edit-input text-dim text-sm ta-center"
          @blur="cards.savePreviewCardField('details')"
          @keydown.enter="($event.target as HTMLInputElement).blur()"
          @keydown.escape="cards.previewCardEditing = null"
        />
      </template>
      <p
        v-else
        class="text-dim text-sm"
        style="cursor: pointer"
        :title="cards.previewCard.details ? 'Double-click to edit' : 'Double-click to set details'"
        @dblclick="cards.startPreviewCardEdit('details')"
      >
        {{ cards.previewCard.details || 'No details' }}
        <span v-if="!cards.previewCard.details" style="opacity: 0.5"> (double-click to edit)</span>
      </p>
    </div>
  </ModalOverlay>
</template>
