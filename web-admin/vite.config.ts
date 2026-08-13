import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

// The admin SPA is served by the Go binary at /admin and embedded via go:embed.
export default defineConfig({
  plugins: [react()],
  base: "/admin/",
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: {
    port: 5174,
    proxy: {
      "/api": "http://localhost:8080",
      "/pubkey": "http://localhost:8080",
    },
  },
  build: {
    outDir: "../server/internal/webadmin/dist", // go:embed target
    emptyOutDir: true,
    // Vite 8 / rolldown auto-splits vendor chunks; an object manualChunks is rejected.
  },
});
