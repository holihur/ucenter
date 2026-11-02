import path from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const target = process.env.VITE_API_TARGET || "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": target,
      "/t": target,
      "/.well-known": target,
    },
  },
  build: {
    outDir: "dist",
    sourcemap: false,
    chunkSizeWarningLimit: 700,
  },
});
