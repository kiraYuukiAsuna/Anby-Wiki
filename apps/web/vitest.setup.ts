import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// vitest 未启用 globals，@testing-library/react 的自动 cleanup 不会注册，需显式执行。
afterEach(() => {
  cleanup();
});

// jsdom 缺少 matchMedia（@blocknote/mantine 的 MantineProvider 需要）。
if (typeof window !== "undefined" && !window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

// jsdom 缺少 ResizeObserver（cmdk 的 Command.List 需要）。
if (typeof window !== "undefined" && !window.ResizeObserver) {
  window.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof window.ResizeObserver;
}

// 当前 jsdom 版本在本环境不提供 localStorage（getter 返回 undefined），
// 编辑会话草稿依赖它；用语义相同的 sessionStorage 兜底。
if (typeof window !== "undefined" && typeof window.localStorage === "undefined") {
  Object.defineProperty(window, "localStorage", {
    value: window.sessionStorage,
    configurable: true,
  });
}
