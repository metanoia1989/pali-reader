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
