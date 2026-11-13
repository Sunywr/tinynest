import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  // envDir '.' resolves against the project root; avoids requiring @types/node in the Vite config.
  const env = loadEnv(mode, '.', 'VITE_')
  return {
    plugins: [vue()],
    server: {
      port: 5173,
      strictPort: true,
      proxy: { '/api': { target: env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8787', changeOrigin: true } },
    },
  }
})
