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
    // Split the rarely-changing dependencies out of the app bundle. The Go server
    // serves /admin/assets/* as immutable (content-hashed), so keeping antd and
    // React in their own chunks means an app-code change re-downloads only the
    // small app chunk — the ~1 MB vendor chunk keeps its hash and stays cached.
    // NB: Vite 8 / rolldown rejects an object-form manualChunks; use the function.
    rolldownOptions: {
      output: {
        advancedChunks: {
          groups: [
            { name: "antd", test: /node_modules[\\/](antd|@ant-design|rc-[^\\/]+)[\\/]/ },
            { name: "react", test: /node_modules[\\/](react|react-dom|react-router|react-router-dom|scheduler)[\\/]/ },
          ],
        },
      },
    },
  },
});
