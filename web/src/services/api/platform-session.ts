import { apiDelete, apiGet, apiPost } from "@/services/api/request";

export type PlatformSessionConfig = {
    appKey: string;
    authorizeUrl: string;
    platformBase: string;
    configured: boolean;
};

export type PlatformSessionStatus = {
    linked: boolean;
    platformUserId: number;
    username: string;
    language: string;
    apiBaseUrl: string;
    authorizeUrl: string;
    quota: number;
    usedQuota: number;
    unlimitedQuota: boolean;
    /** 平台下发的授权项摘要：只有分组、模型与状态，不含任何密钥。 */
    grants: PlatformGrantStatus[];
};

/** 一条平台授权项的对外形态（v3 契约的 grants[]）。 */
export type PlatformGrantStatus = {
    id: string;
    label: string;
    group: string;
    models: string[];
    priority: number;
    status: "active" | "revoked" | string;
};

export type PlatformExchangeResult = {
    token: string;
    user: {
        id: string;
        username: string;
        displayName: string;
        avatarUrl: string;
        role: "guest" | "user" | "admin";
        credits: number;
        createdAt: string;
        updatedAt: string;
    };
};

/** 授权页地址来自后端，前端不出现平台域名。 */
export async function fetchPlatformSessionConfig() {
    return apiGet<PlatformSessionConfig>("/api/session/config");
}

/** 用一次性启动码换本应用会话。兑换后调用方必须立即丢弃 code。 */
export async function exchangePlatformSession(code: string) {
    return apiPost<PlatformExchangeResult>("/api/session/exchange", { code });
}

export async function fetchPlatformStatus(token: string) {
    return apiGet<PlatformSessionStatus>("/api/session", undefined, token);
}

/** 按用户操作触发的额度刷新，不做轮询。 */
export async function refreshPlatformQuota(token: string) {
    return apiPost<PlatformSessionStatus>("/api/session/quota", {}, token);
}

/** 只解除本应用自己的绑定，平台侧密钥保持不动。 */
export async function unlinkPlatformSession(token: string) {
    return apiDelete<boolean>("/api/session/platform", token);
}
