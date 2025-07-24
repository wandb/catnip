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
    port: parseInt(process.env.VITE_PORT || "5173"),
    strictPort: true, // Don't try other ports if configured port is busy
    hmr: {
      // Configure HMR for container development
      host: "localhost",
      port: parseInt(process.env.VITE_PORT || "5173"),
      path: process.env.CATNIP_DEV ? `/${process.env.VITE_PORT}` : undefined, // or whatever path your proxy uses for the socket
      // Use WebSocket for HMR in containers
      protocol: "ws",
    },
  },
  clearScreen: false,
  // Might need this for the proxy to work...
  // base: process.env.CATNIP_DEV ? `/${process.env.VITE_PORT}` : undefined, // or whatever path your proxy uses for the socket
});
