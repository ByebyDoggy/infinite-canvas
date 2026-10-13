import assert from "node:assert/strict";
import test from "node:test";

import { decodePlatformState, encodePlatformState, safeReturnPath } from "./platform-auth";

// state 会经过用户浏览器，可能被截断、手改或换成跨站地址。
test("safeReturnPath only accepts same-site paths", () => {
    assert.equal(safeReturnPath("/canvas/abc"), "/canvas/abc");
    assert.equal(safeReturnPath("/image?a=1"), "/image?a=1");
    for (const rejected of ["//evil.com", "/\\evil.com", "https://evil.com", "http://evil.com", "evil.com", "", "/\t/evil.com", "javascript:alert(1)"]) {
        assert.equal(safeReturnPath(rejected), null, `${rejected} must be rejected`);
    }
});

test("safeReturnPath strips control characters browsers ignore", () => {
    assert.equal(safeReturnPath("/normal\tpath"), "/normalpath");
    assert.equal(safeReturnPath("/normal\npath"), "/normalpath");
});

test("encodePlatformState round-trips returnPath and intent", () => {
    const encoded = encodePlatformState({ returnPath: "/canvas/abc", intent: "generate" });
    const decoded = decodePlatformState(encoded);
    assert.deepEqual(decoded, { returnPath: "/canvas/abc", intent: "generate" });
});

// 解不出来就当没有：一个被手改的 state 不能让整页崩掉。
test("decodePlatformState returns null instead of throwing", () => {
    assert.equal(decodePlatformState(null), null);
    assert.equal(decodePlatformState(undefined), null);
    assert.equal(decodePlatformState(""), null);
    assert.equal(decodePlatformState("not-base64!!!"), null);
    assert.equal(decodePlatformState(btoa("not json")), null);
});

test("decodePlatformState rejects an off-site returnPath", () => {
    const hostile = btoa(JSON.stringify({ v: 1, returnPath: "//evil.com" }));
    assert.equal(decodePlatformState(hostile), null);
});

test("encodePlatformState never emits an off-site returnPath", () => {
    const decoded = decodePlatformState(encodePlatformState({ returnPath: "https://evil.com" }));
    assert.equal(decoded?.returnPath, "/");
});
