import { defineConfig, loadEnv, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// cspPlugin injects a Content-Security-Policy <meta> into index.html for the
// production build only. CSP is the real mitigation for XSS — the attack a
// localStorage-held JWT is exposed to — so locking down where scripts can come
// from and where the app may connect protects the token at its root cause.
// It is build-only so Vite's dev server (inline scripts, eval, ws HMR) keeps
// working; connect-src is derived from VITE_API_URL so only the real API origin
// is allowed.
function cspPlugin(apiOrigin: string): Plugin {
  const csp = [
    "default-src 'self'",
    "script-src 'self'",
    "style-src 'self' 'unsafe-inline'", // Radix/shadcn/sonner set inline styles at runtime
    "img-src 'self' data:",
    "font-src 'self' data:",
    `connect-src 'self' ${apiOrigin}`,
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "object-src 'none'",
  ].join("; ");

  return {
    name: "chillcheck-csp",
    apply: "build",
    transformIndexHtml(html) {
      return {
        html,
        tags: [
          {
            tag: "meta",
            attrs: { "http-equiv": "Content-Security-Policy", content: csp },
            injectTo: "head-prepend",
          },
        ],
      };
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  let apiOrigin = "http://localhost:8080";
  try {
    apiOrigin = new URL(env.VITE_API_URL ?? apiOrigin).origin;
  } catch {
    /* keep default if VITE_API_URL is unset or malformed */
  }

  return {
    plugins: [react(), tailwindcss(), cspPlugin(apiOrigin)],
    resolve: {
      alias: { "@": path.resolve(import.meta.dirname, "./src") },
    },
    server: { port: 5173 },
  };
});
