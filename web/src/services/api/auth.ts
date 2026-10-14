import { apiGet, apiPost } from "@/services/api/request";

export const AUTH_TOKEN_KEY = "infinite-canvas-auth-token-v1";

export type UserRole = "guest" | "user" | "admin";

export type AuthUser = {
    id: string;
    username: string;
    displayName: string;
    avatarUrl: string;
    role: UserRole;
    credits: number;
    createdAt: string;
    updatedAt: string;
};

export type AuthSession = {
    token: string;
    user: AuthUser;
};

// 注册与密码登录只在应用后台（/admin）保留；普通用户的身份一律来自
// New API 平台的一次性启动码，见 services/api/platform-session.ts。
export async function adminLogin(payload: { username: string; password: string }) {
    return apiPost<AuthSession>("/api/admin/login", payload);
}

export async function fetchCurrentUser(token?: string) {
    return apiGet<AuthUser>("/api/auth/me", undefined, token);
}
