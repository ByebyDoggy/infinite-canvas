import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 这些断言读页面源码，钉住的是「只在真浏览器里才会暴露」的规则。
// 上游的广告与自建账号入口一旦被改回来，这里会先红。
const read = (relative: string) =>
    readFileSync(new URL(relative, import.meta.url), "utf8");

test("login page has no self-built account form", () => {
    const source = read("./app/(user)/login/page.tsx");
    assert.ok(!/<Form/.test(source), "登录页不应有表单");
    assert.ok(!/type="password"/.test(source), "登录页不应有密码输入");
    assert.ok(!/register/i.test(source), "登录页不应有注册入口");
    assert.ok(/authorizeUrl/.test(source), "登录页应跳平台授权页");
    assert.ok(!/ai\.lesec\.top/.test(source), "授权页地址不得硬编码平台域名");
});

test("home page carries no upstream advertising", () => {
    const source = read("./app/(user)/page.tsx");
    for (const banned of ["tdeh.top", "metaso.cn", "gcore.jsdelivr.net", "tokendance.space"]) {
        assert.ok(!source.includes(banned), `首页不应出现 ${banned}`);
    }
});

test("top bar carries no upstream links", () => {
    const layout = read("./components/layout/user-status-actions.tsx");
    assert.ok(!/tigerowo/.test(layout), "顶栏不应链到上游仓库");
    const banner = read("./components/layout/announcement-banner.tsx");
    for (const banned of ["tdeh.uk", "tokendance.space", "Metaso"]) {
        assert.ok(!banner.includes(banned), `公告不得含 ${banned}`);
    }
});

test("version check no longer points at the upstream repository", () => {
    const files = [
        "./hooks/use-version-check.ts",
        "./components/layout/version-release-modal.tsx",
    ];
    for (const file of files) {
        try {
            const source = read(file);
            assert.ok(!source.includes("tigerowo"), `${file} 不应引用上游仓库`);
        } catch {
            // 文件已删除，正是期望结果
        }
    }
});

test("codex connect view does not advertise the upstream plugin", () => {
    const source = read("./app/(user)/canvas/components/canvas-codex-connect-view.tsx");
    assert.ok(!/plugin marketplace add/.test(source), "不应再给出上游插件安装命令");
    assert.ok(!/@tigerowo\//.test(source), "不应再引用上游 npm 包");
});
