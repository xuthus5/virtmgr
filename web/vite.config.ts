import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const shellIcons = new Set([
  "activity", "archive", "chevrons-left", "chevrons-right", "command", "corner-down-left", "cpu", "database",
  "hard-drive", "key-round", "layers-3", "layout-dashboard", "layout-template", "list-checks", "loader-circle",
  "log-out", "menu", "moon", "network", "scroll-text", "search", "server", "settings-2", "sun", "users", "x",
]);

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/api": { target: "http://127.0.0.1:8443", ws: true },
      "/healthz": "http://127.0.0.1:8443",
      "/readyz": "http://127.0.0.1:8443",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
    rolldownOptions: {
      output: {
        manualChunks(id) {
          if (/\/src\/features\/[^/]+\/use-[^/]*-query\.ts$/.test(id)) return "query-hooks";
          const icon = id.match(/\/node_modules\/lucide-react\/dist\/esm\/icons\/([^/]+)\.js$/)?.[1];
          if (icon && shellIcons.has(icon)) return "shell-icons";
        },
      },
    },
  },
});
