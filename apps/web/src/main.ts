import { createApp } from 'vue'
import App from './App.vue'
import './styles.css'

createApp(App).mount('#app')

// Service Workers require a secure context (HTTPS or localhost); don't use offline cache for real finance data.
if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {
      // The visible demo is usable even without SW / unsupported browsers.
    })
  })
}
