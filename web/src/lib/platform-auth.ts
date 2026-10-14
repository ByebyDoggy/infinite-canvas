/**
 * New API 应用板块的登录跳转与回调。
 *
 * 三条规则来自平台契约，不能改：
 * 1. 需要身份的动作处才调 requirePlatformLogin，门口不拦——游客本来就能浏览和渲染。
 * 2. state 里必须同时带上 returnPath 与 intent，回来才能「回到原位并继续原动作」。
 * 3. returnPath 会经过用户浏览器，必须做同源校验，否则 state 就是开放重定向入口。
 */

export type PlatformIntent = "generate" | "publish" | "save" | "sync";

export type PlatformResumeState = {
    returnPath: string;
    intent?: PlatformIntent;
};

const STATE_VERSION = 1;

type StatePayload = {
    v: number;
    returnPath: string;
    intent?: PlatformIntent;
};

/** 浏览器与 Node 都要能跑，所以不依赖 window。 */
function encodeBase64(value: string): string {
    if (typeof btoa === "function") {
        return btoa(String.fromCharCode(...new TextEncoder().encode(value)));
    }
    return Buffer.from(value, "utf-8").toString("base64");
}

function decodeBase64(value: string): string {
    if (typeof atob === "function") {
        return new TextDecoder().decode(Uint8Array.from(atob(value), (char) => char.charCodeAt(0)));
    }
    return Buffer.from(value, "base64").toString("utf-8");
}

/** 只接受站内路径。浏览器会忽略 Tab/换行/回车，并把 //host、/\host 解析成跨站地址。 */
export function safeReturnPath(target: unknown): string | null {
    if (typeof target !== "string") return null;
    const cleaned = target.replace(/[\t\n\r]/g, "");
    if (!cleaned.startsWith("/") || cleaned.startsWith("//") || cleaned.startsWith("/\\")) return null;
    return cleaned;
}

/**
 * 解 state。它经过用户浏览器，可能被截断或手改，
 * 所以解不出来就返回 null，绝不让整页崩掉。
 */
export function decodePlatformState(raw: string | null | undefined): PlatformResumeState | null {
    if (!raw) return null;
    try {
        const parsed = JSON.parse(decodeBase64(raw)) as StatePayload;
        const returnPath = safeReturnPath(parsed?.returnPath);
        if (!returnPath) return null;
        return { returnPath, intent: parsed?.intent };
    } catch {
        return null;
    }
}

export function encodePlatformState(state: PlatformResumeState): string {
    const payload: StatePayload = {
        v: STATE_VERSION,
        returnPath: safeReturnPath(state.returnPath) || "/",
        ...(state.intent ? { intent: state.intent } : {}),
    };
    return encodeBase64(JSON.stringify(payload));
}

/**
 * 需要身份的动作处调用。返回 false 表示已经跳走，调用方必须立刻停手。
 */
export function requirePlatformLogin(intent: PlatformIntent, authorizeUrl: string, appKey: string): boolean {
    if (!authorizeUrl || !appKey) return false;
    const state = encodePlatformState({
        returnPath: `${window.location.pathname}${window.location.search}`,
        intent,
    });
    window.location.href = `${authorizeUrl}?app=${encodeURIComponent(appKey)}&state=${encodeURIComponent(state)}`;
    return false;
}

/**
 * 拼平台托管授权页地址。授权页地址只来自后端（`/api/session/config` 或 exchange
 * 返回的 `authorize_url`），前端不硬编码平台域名——换一个部署就废。
 */
export function buildPlatformAuthorizeURL(
    authorizeUrl: string,
    appKey: string,
    returnPath: string
): string {
    const state = encodePlatformState({ returnPath: safeReturnPath(returnPath) || "/" });
    return `${authorizeUrl}?app=${encodeURIComponent(appKey)}&state=${encodeURIComponent(state)}`;
}


/** 回调地址里的 code 是 5 分钟有效的凭据，兑换后立刻从地址栏抹掉。 */
export function stripCallbackParams() {
    const params = new URLSearchParams(window.location.search);
    ["code", "state"].forEach((key) => params.delete(key));
    const query = params.toString();
    window.history.replaceState(null, "", `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`);
}
