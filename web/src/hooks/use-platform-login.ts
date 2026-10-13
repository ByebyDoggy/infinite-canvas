"use client";

import { requirePlatformLogin, type PlatformIntent } from "@/lib/platform-auth";
import { usePlatformStore } from "@/stores/use-platform-store";
import { useUserStore } from "@/stores/use-user-store";

/**
 * 判断当前是否需要先登录平台。
 *
 * 只在**真正要调模型的那一刻**调用，门口不拦——首页、画布、素材库的渲染
 * 本来就不需要身份。返回 true 表示已有会话，调用方可以继续；返回 false
 * 表示已经把浏览器送去授权页，调用方必须立刻停手。
 */
export function ensurePlatformSession(intent: PlatformIntent): boolean {
    const token = useUserStore.getState().token;
    if (token) return true;
    const config = usePlatformStore.getState().config;
    // 平台未配置时退回本应用自己的登录页，避免把用户送到一个空地址。
    if (!config?.configured || !config.authorizeUrl || !config.appKey) {
        if (typeof window !== "undefined") {
            window.location.href = `/login?redirect=${encodeURIComponent(window.location.pathname + window.location.search)}`;
        }
        return false;
    }
    return requirePlatformLogin(intent, config.authorizeUrl, config.appKey);
}

/** 给 React 组件用的同名 hook，语义与 ensurePlatformSession 一致。 */
export function usePlatformLogin() {
    return ensurePlatformSession;
}
