import { create } from "zustand";

/**
 * 本地交互状态占位示例：侧栏面板开关。
 * 仅存放 UI 交互状态；远端数据一律走 SWR，不要放进这里。
 */
interface UiState {
  sidebarOpen: boolean;
  toggleSidebar: () => void;
  setSidebarOpen: (open: boolean) => void;
}

export const useUiStore = create<UiState>((set) => ({
  sidebarOpen: true,
  toggleSidebar: () => set((state) => ({ sidebarOpen: !state.sidebarOpen })),
  setSidebarOpen: (open) => set({ sidebarOpen: open }),
}));
