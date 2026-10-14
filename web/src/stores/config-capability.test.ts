import assert from "node:assert/strict";
import test from "node:test";

import { modelMatchesCapability } from "@/stores/use-config-store";

// 平台模型列表直接来自 New API，模型名由上游决定。这里钉住几个真实的
// gpt-image 系列名字都能被认成生图模型——认错会让生图台的下拉框是空的。
test("gpt-image family resolves to the image capability", () => {
    for (const model of ["gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "gpt-image-1.5"]) {
        assert.equal(modelMatchesCapability(model, "image", "openai"), true, model);
    }
});

test("text models stay out of the image capability", () => {
    for (const model of ["gpt-5.5", "gpt-6-astra", "deepseek-v4.1-flash"]) {
        assert.equal(modelMatchesCapability(model, "image", "openai"), false, model);
    }
});

test("video and audio models never claim the image capability", () => {
    assert.equal(modelMatchesCapability("sora-2", "image", "openai"), false);
    assert.equal(modelMatchesCapability("gpt-4o-mini-tts", "image", "openai"), false);
});
