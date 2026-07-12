import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      // cmdk drags in the full @radix-ui/react-dialog tree solely for its unused
      // Command.Dialog wrapper (we use cmdk's `Command` standalone, wrapped in the
      // app's own native-<dialog> shell — see src/shims/radix-dialog-stub.tsx).
      '@radix-ui/react-dialog': fileURLToPath(new URL('./src/shims/radix-dialog-stub.tsx', import.meta.url)),
    },
  },
})
