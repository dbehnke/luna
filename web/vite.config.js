import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    // Current largest vendor chunk is hls.js; keep warning threshold practical for this app.
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes("node_modules/hls.js")) return "vendor-hls";
          if (id.includes("node_modules/vue-router")) return "vendor-router";
          if (id.includes("node_modules")) return "vendor";
        },
      },
    },
  },
});
