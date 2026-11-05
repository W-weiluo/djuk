package app

// 网页系站源(禁果等)共用的校验与播放地址准备辅助,移植自 guoapp provider_web.go。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var webProviderNumericID = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)

func webProviderInteger(value any, maximum int) (int, bool) {
	number, err := strconv.Atoi(nativeText(value))
	return number, err == nil && number >= 0 && number <= maximum
}

func (d *Downloader) prepareWebProviderMedia(ctx context.Context, media providerMedia, sourceName string) (providerMedia, error) {
	address, err := url.Parse(media.URL)
	if err != nil || !isProviderHTTPMediaURL(media.URL) || address.User != nil {
		return providerMedia{}, fmt.Errorf("%s未返回有效播放地址，请刷新详情后重试", sourceName)
	}
	if strings.HasSuffix(strings.ToLower(address.Path), ".m3u8") {
		playlist, err := d.fetchProviderText(ctx, media.URL, media.Referer)
		if err != nil {
			return providerMedia{}, fmt.Errorf("获取%s播放列表失败：%w", sourceName, err)
		}
		if !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(playlist, "\ufeff")), "#EXTM3U") {
			return providerMedia{}, fmt.Errorf("%s播放列表无效，请重新解析播放", sourceName)
		}
		if duration := m3u8Duration(playlist); duration > 0 {
			media.Duration = duration
		}
		media.Playlist = playlist
	}
	return media, nil
}

func nativeText(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"url", "src", "cover", "image", "pic"} {
			if text := nativeText(v[key]); text != "" {
				return text
			}
		}
	case []any:
		for _, entry := range v {
			if text := nativeText(entry); text != "" {
				return text
			}
		}
	case json.Number:
		return string(v)
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	}
	return ""
}

func firstPresent(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok && value != nil && nativeText(value) != "" {
			return value
		}
	}
	return nil
}
