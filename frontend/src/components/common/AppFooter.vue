<script setup lang="ts">
/**
 * Site footer shown on the public (non-admin) pages — credits + external links,
 * plus a mobile-only "Install App" link that adds the PWA to the home screen.
 */
import { usePwaInstall } from '@/composables/usePwaInstall'
import { useUiStore } from '@/stores/ui'

const { showInstall, canPrompt, needsIosInstructions, promptInstall } = usePwaInstall()
const ui = useUiStore()

/**
 * Installs the PWA. On Chromium with a captured prompt we fire the native
 * dialog; otherwise we surface manual steps — iOS uses the Share sheet, while
 * other browsers without `beforeinstallprompt` (Vivaldi, Firefox, …) install
 * from their own menu.
 */
async function onInstall(): Promise<void> {
  if (canPrompt.value) {
    const outcome = await promptInstall()
    if (outcome === 'accepted') ui.notify('Installing — check your home screen!', 'success')
    return
  }
  if (needsIosInstructions.value) {
    ui.notify("To install: tap the Share button, then 'Add to Home Screen'.", 'info', 9000)
  } else {
    ui.notify(
      "To install: open your browser menu, then 'Install app' or 'Add to Home Screen'.",
      'info',
      9000,
    )
  }
}
</script>

<template>
  <footer class="app-footer">
    <p>
      Created for
      <a href="https://senpan.cafe" target="_blank" rel="noopener">Senpan Tea House</a>
      and
      <a href="https://atelieryao.crd.co/#home" target="_blank" rel="noopener">Atelier Yao</a>.
    </p>
    <p>
      Developed by
      <a href="https://xiv.mathdad.me" target="_blank" rel="noopener">MathDad</a>
    </p>
    <p v-if="showInstall" class="app-footer-install">
      <br />
      <a href="#" role="button" @click.prevent="onInstall">
        <font-awesome-icon :icon="['fas', 'download']" />
        Install App
      </a>
    </p>
  </footer>
</template>
