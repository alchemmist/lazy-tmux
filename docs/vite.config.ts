import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The site is served from a custom domain (lazy-tmux.xyz), so the base path is
// the root. install.sh, CNAME and assets/ live in public/ and Vite copies them
// verbatim to the dist root, preserving https://lazy-tmux.xyz/install.sh etc.
export default defineConfig({
  base: "/",
  plugins: [react()],
  build: {
    outDir: "dist",
  },
  // "nested" emits /features/index.html (not /features.html) so the existing
  // trailing-slash canonical URLs like https://lazy-tmux.xyz/features/ resolve.
  ssgOptions: {
    dirStyle: "nested",
  },
  // Gravity UI's ESM build has CSS side-effect imports inside its component
  // files. During SSG the server bundle runs in Node, which can't import .css,
  // so bundle the Gravity packages through Vite instead of externalizing them.
  ssr: {
    noExternal: [/@gravity-ui\//],
  },
});
