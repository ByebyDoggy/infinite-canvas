"use client";

import type { ReactNode } from "react";
import { useEffect, useLayoutEffect, useRef } from "react";
import { usePathname } from "next/navigation";
import { App } from "antd";

import { fetchUserConfig } from "@/services/api/user-config";
import { exchangePlatformSession, fetchPlatformSessionConfig, type PlatformSessionConfig } from "@/services/api/platform-session";
import { replaceWorkflowChannels } from "@/services/workflow-channel-storage";
import { STORAGE_SYNC_FAILED_EVENT, defaultUserStorageProvider, defaultUserWebDAVStorageProvider, saveUserStorageProvider, saveUserWebDAVStorageProvider } from "@/services/image-storage";
import { decodePlatformState, safeReturnPath, stripCallbackParams } from "@/lib/platform-auth";
import { defaultConfig, useConfigStore, type AiConfig } from "@/stores/use-config-store";
import { usePlatformStore } from "@/stores/use-platform-store";
import { useUserStore } from "@/stores/use-user-store";
import { useChannelTranslationStore } from "@/stores/use-channel-translation-store";

export function ClientRootInit({ children }: { children: ReactNode }) {
    const { message } = App.useApp();
    const handledConfigParams = useRef(false);
    const handledPlatformCallback = useRef(false);
    const setPlatformConfig = usePlatformStore((state) => state.setConfig);
    const pathname = usePathname();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const hydrateUser = useUserStore((state) => state.hydrateUser);
    const loadPublicSettings = useConfigStore((state) => state.loadPublicSettings);
    const publicSettings = useConfigStore((state) => state.publicSettings);
    const channelMode = useConfigStore((state) => state.config.channelMode);
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const isLoginPage = pathname === "/login" || pathname === "/admin/login";
    const isTokenDanceCallback = pathname === "/tokendance/callback";
    const adminRemoteTokenRef = useRef("");
    const accountSessionRef = useRef({ token, userId: user?.id || "" });

    useEffect(() => {
        const onSyncFailed = (event: Event) => {
            const detail = (event as CustomEvent<string>).detail;
            message.warning({ key: STORAGE_SYNC_FAILED_EVENT, content: `云端同步失败，已保留原始素材${detail ? `：${detail}` : ""}` });
        };
        window.addEventListener(STORAGE_SYNC_FAILED_EVENT, onSyncFailed);
        return () => window.removeEventListener(STORAGE_SYNC_FAILED_EVENT, onSyncFailed);
    }, [message]);

    // 平台用户的模型列表由服务端按身份注入，身份一变换就得重取一次；
    // 否则用户登录后仍会用着游客那份配置，模型列表是空的。
    const settingsIdentityRef = useRef("");
    useEffect(() => {
        const identity = token || "guest";
        if (settingsIdentityRef.current === identity) return;
        settingsIdentityRef.current = identity;
        void loadPublicSettings();
    }, [loadPublicSettings, token]);

    useEffect(() => {
        if (!isLoginPage) void hydrateUser();
    }, [hydrateUser, isLoginPage]);

	useEffect(() => {
		if (!token || adminRemoteTokenRef.current === token) return;
		if (isTokenDanceCallback) {
			adminRemoteTokenRef.current = token;
			return;
		}
		if (user?.role !== "admin") return;
		adminRemoteTokenRef.current = token;
		if (channelMode !== "remote") updateConfig("channelMode", "remote");
	}, [channelMode, isTokenDanceCallback, token, updateConfig, user?.role]);

	useLayoutEffect(() => {
		const previous = accountSessionRef.current;
		const userId = user?.id || "";
		if ((previous.token && !token) || (previous.userId && userId && previous.userId !== userId)) {
			useConfigStore.setState({ config: defaultConfig });
		} else {
			updateConfig("workflowSyncTouched", false);
		}
		accountSessionRef.current = { token, userId };
	}, [token, updateConfig, user?.id]);

	useEffect(() => {
        if (isTokenDanceCallback || !token || !user?.id) return;
        const accountToken = token;
        const accountId = user.id;
        const translationRevision = useChannelTranslationStore.getState().revisions[accountId] || 0;
        let canceled = false;
		void fetchUserConfig(accountToken)
			.then(async (payload) => {
				if (canceled || useUserStore.getState().token !== accountToken || useUserStore.getState().user?.id !== accountId) return;
				let workflowsReady = true;
				const syncS3 = payload.modelConfig?.syncStorageConfig === true;
                const syncWebDAV = payload.modelConfig?.syncWebDAVStorageConfig === true;
                if (payload.modelConfig) {
                    const { workflowChannels, channelTranslations, ...modelConfig } = payload.modelConfig;
					await useChannelTranslationStore.getState().replace(accountId, channelTranslations || [], translationRevision);
					if (canceled || useUserStore.getState().token !== accountToken || useUserStore.getState().user?.id !== accountId) return;
					if (workflowChannels !== undefined) {
						try {
							await replaceWorkflowChannels(accountId, workflowChannels);
						} catch {
							workflowsReady = false;
						}
					}
                    if (canceled || useUserStore.getState().token !== accountToken || useUserStore.getState().user?.id !== accountId) return;
					Object.entries(modelConfig)
						.forEach(([key, value]) => updateConfig(key as keyof AiConfig, value as never));
				}
				updateConfig("workflowSyncTouched", workflowsReady);
				updateConfig("syncStorageConfig", syncS3);
                updateConfig("syncWebDAVStorageConfig", syncWebDAV);
                if (syncS3 && payload.storageProvider?.s3) {
                    saveUserStorageProvider({
                        ...defaultUserStorageProvider(),
                        ...payload.storageProvider.s3,
                        type: "s3",
                    });
                }
                if (syncWebDAV && payload.storageProvider?.webdav) {
                    saveUserWebDAVStorageProvider({
                        ...defaultUserWebDAVStorageProvider(),
                        ...payload.storageProvider.webdav,
                        type: "webdav",
                    });
                }
            })
            .catch(() => {});
        return () => {
            canceled = true;
        };
    }, [isTokenDanceCallback, token, updateConfig, user?.id]);

    useEffect(() => {
        if (handledConfigParams.current) return;
        const searchParams = new URLSearchParams(window.location.search);
        const baseUrl = searchParams.get("baseUrl") || searchParams.get("baseurl");
        const apiKey = searchParams.get("apiKey") || searchParams.get("apikey");
        if (!baseUrl && !apiKey) return;
        if (!publicSettings) return;
        handledConfigParams.current = true;
        searchParams.delete("baseUrl");
        searchParams.delete("baseurl");
        searchParams.delete("apiKey");
        searchParams.delete("apikey");
        window.history.replaceState(null, "", `${window.location.pathname}${searchParams.size ? `?${searchParams}` : ""}${window.location.hash}`);
        if (!publicSettings.modelChannel.allowCustomChannel) {
            openConfigDialog(false);
            message.error("后台未允许用户自定义渠道，请联系管理员进行配置");
            return;
        }
        updateConfig("channelMode", "local");
        if (baseUrl) updateConfig("baseUrl", baseUrl);
        if (apiKey) updateConfig("apiKey", apiKey);
        openConfigDialog(false);
    }, [message, openConfigDialog, publicSettings, updateConfig]);

    // New API 应用启动码回调：解 state → 换会话 → 按 returnPath 跳回 → 按 intent 继续。
    // 顺序不能反，反了就是「登录回来落到首页」，用户会以为白登一次。
    useEffect(() => {
        if (handledPlatformCallback.current) return;
        const searchParams = new URLSearchParams(window.location.search);
        const code = searchParams.get("code");
        if (!code) return;
        handledPlatformCallback.current = true;
        const resume = decodePlatformState(searchParams.get("state"));
        // code 是 5 分钟有效的凭据，先从地址栏抹掉再兑换。
        stripCallbackParams();
        void (async () => {
            try {
                const session = await exchangePlatformSession(code);
                useUserStore.getState().setSession(session.token, session.user);
                const target = safeReturnPath(resume?.returnPath);
                if (target) {
                    window.location.replace(target);
                    return;
                }
                message.success("已连接 New API 账号");            } catch (error) {
                // 兑换失败回首页，不回死胡同：这次没登上不等于应用不能用。
                message.error(error instanceof Error ? error.message : "登录失败，请联系平台管理员");
            }
        })();
    }, [message]);

    // 授权页地址来自后端；首次接入还没调过 exchange，所以后端有配置兜底。
    useEffect(() => {
        void fetchPlatformSessionConfig()
            .then((config) => setPlatformConfig(config))
            .catch(() => {});
    }, []);

    return <>{children}</>;
}
