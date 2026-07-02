<script setup lang="ts">
/**
 * Global themed confirmation dialog — a styled replacement for window.confirm.
 *
 * Driven entirely by the `ui` store: `ui.confirm(message, opts)` opens it and
 * resolves the returned promise when the user picks Confirm/Cancel. Rendered
 * once in App.vue so it's available from every view. Because it uses the
 * standard `.modal-overlay` / `.modal-box` markup and button classes, custom
 * themes restyle it like the rest of the site.
 */
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()
</script>

<template>
  <ModalOverlay v-if="ui.confirmState.show" centered @close="ui.resolveConfirm(false)">
    <h3>{{ ui.confirmState.title }}</h3>
    <p class="confirm-msg">{{ ui.confirmState.message }}</p>
    <div class="confirm-btns">
      <button class="btn-neutral" @click="ui.resolveConfirm(false)">
        {{ ui.confirmState.cancelText }}
      </button>
      <button
        :class="ui.confirmState.danger ? 'btn-danger' : 'btn-confirm'"
        @click="ui.resolveConfirm(true)"
      >
        {{ ui.confirmState.confirmText }}
      </button>
    </div>
  </ModalOverlay>
</template>
