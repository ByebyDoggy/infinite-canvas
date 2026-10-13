"use client";

import { create } from "zustand";

import type { PlatformSessionConfig } from "@/services/api/platform-session";

type PlatformStore = {
    /** 授权页地址与 app key 一律来自后端，前端不硬编码平台域名。 */
    config: PlatformSessionConfig | null;
    setConfig: (config: PlatformSessionConfig) => void;
    /** 登录门槛发生在动作处，这里只记录被拦下的意图，回来后继续。 */
    pendingIntent: string;
    setPendingIntent: (intent: string) => void;
    clearPendingIntent: () => void;
};

export const usePlatformStore = create<PlatformStore>()((set) => ({
    config: null,
    setConfig: (config) => set({ config }),
    pendingIntent: "",
    setPendingIntent: (pendingIntent) => set({ pendingIntent }),
    clearPendingIntent: () => set({ pendingIntent: "" }),
}));
