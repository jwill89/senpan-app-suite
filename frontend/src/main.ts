/**
 * Application entry point.
 *
 * Creates the Vue app, installs Pinia + Vue Router, initializes FontAwesome (SVG
 * auto-replacement of <i class="fa-..."> markup), and mounts to #app.
 */
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { initFontAwesome } from './lib/fontawesome'
import { useUiStore } from './stores/ui'

// Global app styles. Imported here (rather than linked statically from public/)
// so Vite content-hashes the emitted CSS for cache-busting. Vite injects the
// hashed <link> into <head>; the runtime custom-theme <style> (applyCustomCSS)
// is appended to <head> afterward, so theme overrides still cascade correctly.
import './assets/app.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)

// Surface otherwise-silent runtime errors as a toast (and keep logging them to
// the console) so failures are visible instead of leaving the UI half-rendered.
app.config.errorHandler = (err, _instance, info) => {
  console.error('[app error]', err, info)
  try {
    useUiStore().notify('Something went wrong. Please try again.', 'error')
  } catch {
    /* Pinia not ready / store unavailable — console log above is the fallback. */
  }
}

app.mount('#app')

// Start FontAwesome DOM watching after mount so initial icons are replaced and
// any icons rendered by later Vue updates are kept in sync.
initFontAwesome()
