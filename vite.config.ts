import path from "path"
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import tailwindcss from '@tailwindcss/vite'

import { cloudflare } from "@cloudflare/vite-plugin";

const plugins = [react(), tailwindcss()]

if (process.env.CLOUDFLARE_DEV === "true") {
  plugins.push(cloudflare())
}

// https://vite.dev/config/
export default defineConfig({
  plugins,
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  }
})