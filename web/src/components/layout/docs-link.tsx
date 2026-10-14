"use client";

import { BookOutlined } from "@ant-design/icons";

import { cn } from "@/lib/utils";

type DocsLinkProps = {
    className?: string;
    style?: React.CSSProperties;
};

// 上游的 GitHub 入口已移除：本应用是 New API 的接入方，用户不需要、
// 也不应该被引到上游仓库。这里保留一个指向本站提示词库的入口，
// 避免顶栏少一个图标导致布局塌陷。
export function DocsLink({ className, style }: DocsLinkProps) {
    return (
        <a
            className={cn("inline-flex size-9 shrink-0 items-center justify-center rounded-full text-stone-600 transition hover:bg-stone-100 hover:text-stone-950 dark:text-stone-300 dark:hover:bg-stone-800 dark:hover:text-white", className)}
            style={style}
            href="/prompts"
            aria-label="提示词库"
            title="提示词库"
        >
            <BookOutlined className="text-base" />
        </a>
    );
}
