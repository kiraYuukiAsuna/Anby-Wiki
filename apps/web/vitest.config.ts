import react from "@vitejs/plugin-react";
import tsconfigPaths from "vite-tsconfig-paths";
import { configDefaults, defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react(), tsconfigPaths()],
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    // Playwright 的 *.spec.ts 由独立 browser-e2e Job 执行，不能被 Vitest 加载。
    exclude: [...configDefaults.exclude, "e2e/**"],
  },
});
