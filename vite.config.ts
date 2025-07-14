import path from "path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import tailwindcss from "@tailwindcss/vite";

import { cloudflare } from "@cloudflare/vite-plugin";

const plugins = [
  react(),
  tailwindcss(),
  tanstackRouter({
    target: "react",
    autoCodeSplitting: true,
  }),
];

if (process.env.CLOUDFLARE_DEV === "true") {
  plugins.push(cloudflare());
}

// https://vite.dev/config/
export default defineConfig({
  plugins,
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    host: "0.0.0.0", // Allow external connections
    port: 5173,
    strictPort: true, // Don't try other ports if 5173 is busy
    hmr: {
      // Configure HMR for container development
      host: "localhost",
      port: 5173,
      // Use WebSocket for HMR in containers
      protocol: "ws",
    },
  },
});
