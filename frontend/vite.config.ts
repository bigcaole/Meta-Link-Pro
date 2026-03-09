import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import wails from '@wailsio/runtime/plugins/vite'

export default defineConfig({
  plugins: [vue(), wails('./bindings')],
  server: {
    port: 5173,
    strictPort: true
  }
})
