"use client";

import { Button } from "antd";
import { useRouter } from "next/navigation";
import { Suspense, useEffect, useRef } from "react";

import { buildPlatformAuthorizeURL, safeReturnPath } from "@/lib/platform-auth";
import { usePlatformStore } from "@/stores/use-platform-store";
import { useUserStore } from "@/stores/use-user-store";

// 过渡页：本应用没有登录表单，唯一的进门方式是 New API 平台签发的一次性启动码。
// 保留这个路由是因为旧链接、书签和应用内部跳转都指向 /login，直接 404 会让用户
// 以为站点坏了。这里只把浏览器送到平台托管授权页。
export default function LoginPage() {
    return (
        <Suspense fallback={null}>
            <PlatformRedirect />
        </Suspense>
    );
}

function PlatformRedirect() {
    const router = useRouter();
    const started = useRef(false);
    const platformConfig = usePlatformStore((state) => state.config);
    const setPlatformConfig = usePlatformStore((state) => state.setConfig);

    useEffect(() => {
        if (started.current) return;
        started.current = true;
        void (async () => {
            if (useUserStore.getState().token) {
                router.replace("/");
                return;
            }
            let config = platformConfig;
            if (!config) {
                try {
                    const { fetchPlatformSessionConfig } = await import("@/services/api/platform-session");
                    config = await fetchPlatformSessionConfig();
                    setPlatformConfig(config);
                } catch {
                    config = null;
                }
            }
            if (!config?.authorizeUrl || !config.appKey) {
                router.replace("/");
                return;
            }
            const returnPath = safeReturnPath(new URLSearchParams(window.location.search).get("redirect")) || "/";
            window.location.replace(buildPlatformAuthorizeURL(config.authorizeUrl, config.appKey, returnPath));
        })();
    }, [platformConfig, router, setPlatformConfig]);

    return (
        <main className="flex h-full min-h-0 items-center justify-center bg-background px-6">
            <div className="text-center">
                <span
                    className="mx-auto mb-4 block size-12 animate-pulse bg-stone-950 dark:bg-stone-100"
                    style={{
                        mask: "url(/logo.svg) center / contain no-repeat",
                        WebkitMask: "url(/logo.svg) center / contain no-repeat",
                    }}
                    aria-label="无限画布"
                />
                <p className="text-sm text-stone-500 dark:text-stone-400">正在跳转到登录页……</p>
                <Button className="mt-4" onClick={() => router.replace("/")}>
                    返回首页
                </Button>
            </div>
        </main>
    );
}
