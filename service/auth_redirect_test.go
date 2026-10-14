package service

import (
	"testing"
)

// safeRedirectPath 仍在使用：RequestOrigin 拼出的回跳地址、
// 以及后台登录成功后回 /admin 的路径都要过这一关。
func TestSafeRedirectPath(t *testing.T) {
	cases := map[string]string{
		"/":                    "/",
		"/canvas/abc":          "/canvas/abc",
		"/login?redirect=/x":   "/login?redirect=/x",
		"":                     "/",
		"//evil.com":           "/",
		"/\\evil.com":          "/",
		"https://evil.com":     "/",
		"http://evil.com":      "/",
		"javascript:alert(1)":  "/",
		"evil.com":             "/",
		"/\t/evil.com":         "/", // browsers strip the tab → //evil.com
		"/normal\tpath":        "/normalpath",
	}
	for in, want := range cases {
		if got := safeRedirectPath(in); got != want {
			t.Errorf("safeRedirectPath(%q) = %q, want %q", in, got, want)
		}
	}
}
