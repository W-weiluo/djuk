package app

// 红果 provider 整体移植自 Xanerix/guoapp(2026-10-27)。本文件保留 duanjuku
// 自有代码(library 缓存、封面、排序、榜单)依赖、而 guoapp 版本未提供的符号。

import (
	"context"
	"net/url"
	"strings"
)

type requestLaneKey struct{}

type nativeCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func withKnownHongguoDramas(ctx context.Context, dramas []Drama) context.Context {
	known := make(map[string]bool, len(dramas))
	for _, drama := range dramas {
		known[drama.ID] = true
	}
	ctx = context.WithValue(ctx, libraryRowsKey{}, append([]Drama(nil), dramas...))
	return context.WithValue(ctx, libraryKnownKey{}, known)
}

func hongguoCoverAddress(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 8192 {
			continue
		}
		parsed, err := url.Parse(value)
		if err == nil && validImageURL(parsed) {
			return value
		}
	}
	return ""
}
