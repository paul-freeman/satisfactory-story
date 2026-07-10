import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const backend = 'http://localhost:28100'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/state': backend,
      '/tick': backend,
      '/run': backend,
      '/stop': backend,
      '/reset': backend,
      '/recipes': backend,
      '/recipe': backend,
    },
  },
})
