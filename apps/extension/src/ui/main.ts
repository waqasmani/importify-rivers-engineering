import { createApp } from 'vue'
import { z } from 'zod'
import App from './App.vue'
import './style.css'

z.config({ jitless: true })

const surface = document.body.dataset['surface'] === 'popup' ? 'popup' : 'sidepanel'
createApp(App, { surface }).mount('#app')
