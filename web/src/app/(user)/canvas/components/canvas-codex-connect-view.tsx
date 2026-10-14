"use client";

import { useEffect, useState } from "react";
import { Button, Input, Tooltip, Typography } from "antd";
import { KeyRound, Link2, MessageSquare, PlugZap } from "lucide-react";
import { canvasThemes } from "@/lib/canvas-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import type { useCodexAgent } from "../agent/use-codex-agent";

// 这里只保留「连接本地已有 Agent」这一段。
//
// 原来的「方式一：在 Codex 中使用插件」把安装命令写死成第三方分发的插件源
// （上游作者的 GitHub 仓库），与本部署无关，已移除。需要装插件的用户请让
// 管理员提供本站自己的插件源，不要把那串上游地址塞回页面。
export function CanvasCodexConnectView({ agent, onChat }: {
    agent: Pick<ReturnType<typeof useCodexAgent>, "connection" | "status" | "error" | "connect" | "disconnect">;
    onChat: () => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const [draft, setDraft] = useState(agent.connection);
    const connected = agent.status === "ready";
    const connecting = agent.status === "connecting";
    const statusText = { idle: "未连接", connecting: "连接中", ready: "已连接", error: "连接失败" }[agent.status];
    useEffect(() => setDraft(agent.connection), [agent.connection.endpoint, agent.connection.token]);
    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-base font-semibold leading-6">连接本地 Agent</h2>
                <p className="mt-1 text-xs leading-5" style={{ color: theme.node.muted }}>填入本地 Agent 的地址与连接 Token 即可接入。</p>
            </div>
            <div className="rounded-lg border p-3" style={{ borderColor: theme.node.stroke }}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                            <span className="text-sm font-medium">网页连接</span>
                            <span className="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px]" style={{ borderColor: theme.node.stroke, color: connected ? theme.node.text : theme.node.muted }}><span className="size-1.5 rounded-full bg-current" />{statusText}</span>
                        </div>
                        <p className="mt-1 text-xs leading-5" style={{ color: theme.node.muted }}>使用已保存的连接信息，也可手动填写。</p>
                    </div>
                    <Button type={connected || connecting ? "default" : "primary"} icon={<PlugZap className="size-4" />} onClick={async () => {
                        if (connected || connecting) agent.disconnect();
                        else if (await agent.connect(draft)) onChat();
                    }}>{connected || connecting ? "断开" : "连接"}</Button>
                </div>
                <div className="mt-3 grid gap-3">
                    <label className="grid gap-1.5">
                        <span className="flex items-center gap-1.5 text-xs" style={{ color: theme.node.muted }}><Link2 className="size-3.5" />本地地址 <span className="opacity-70">Local URL</span></span>
                        <Input size="large" prefix={<Link2 className="mr-1 size-4" style={{ color: theme.node.faint }} />} aria-label="本地 Agent 地址" value={draft.endpoint} onChange={(event) => setDraft((current) => ({ ...current, endpoint: event.target.value }))} placeholder="http://127.0.0.1:3210" />
                    </label>
                    <label className="grid gap-1.5">
                        <span className="flex items-center gap-1.5 text-xs" style={{ color: theme.node.muted }}><KeyRound className="size-3.5" />连接 Token <span className="opacity-70">Connect token</span></span>
                        <Input.Password size="large" prefix={<KeyRound className="mr-1 size-4" style={{ color: theme.node.faint }} />} aria-label="连接 Token" value={draft.token} onChange={(event) => setDraft((current) => ({ ...current, token: event.target.value }))} placeholder="粘贴本地 Agent 输出的 Connect token" />
                    </label>
                    {agent.error ? <Typography.Text type="danger" role="alert" className="!text-xs leading-5">{agent.error}</Typography.Text> : null}
                    {connected ? <Button type="primary" icon={<MessageSquare className="size-4" />} onClick={onChat}>进入 Codex 对话</Button> : null}
                </div>
            </div>
        </div>
    );
}
