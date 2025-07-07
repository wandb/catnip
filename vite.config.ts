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

// Detect if we're running in a container (Catnip development environment)
const isContainer = process.env.CATNIP_DEV === "true";

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
      host: isContainer ? "localhost" : "localhost",
      port: 5173,
      // Use WebSocket for HMR in containers
      protocol: "ws",
    },
    /*watch: {
      // Use polling for better file watching in containers/Docker
      usePolling: isContainer,
      interval: isContainer ? 1000 : undefined, // Check for changes every second in containers
    }*/
  },
});
