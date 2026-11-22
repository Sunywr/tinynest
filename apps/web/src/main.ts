import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import './styles.css'

createApp(App).use(router).mount('#app')

// Service Workers require a secure context (HTTPS or localhost); the bookkeeping
// shell is network-first so offline mode never pretends a write was saved.
if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {
      // The app stays usable even without SW / in unsupported browsers.
    })
  })
}
