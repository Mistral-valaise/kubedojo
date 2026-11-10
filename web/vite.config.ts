import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev: `npm run dev` proxies the API (and the terminal WebSocket) to a locally
// running `kubedojo serve`. Build: output is embedded into the Go binary.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: "../internal/webui/dist", emptyOutDir: true, chunkSizeWarningLimit: 1200 },
  server: { port: 5173, proxy: { "/api": { target: "http://127.0.0.1:4400", ws: true } } },
});
