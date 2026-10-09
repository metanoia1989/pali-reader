import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useSettings } from './store/settings'
import './assets/base.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)

// Apply the stored display settings before the first paint, so a reader who
// chose 特大 never sees a frame of the default size.
useSettings().apply()

app.mount('#app')

// The service worker caches the built shell only — see public/sw.js for what it
// refuses to touch and why. Registered after load so it never competes with the
// first paint, and skipped in dev, where the files it would cache are not the
// ones being served.
if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {
      // A reader without the worker is a reader with a slightly slower repeat
      // visit. Not a reason to say anything.
    })
  })
}
