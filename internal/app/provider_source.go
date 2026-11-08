package app

import (
	"strings"
)

const (
	sourceHuangguoAI    = "huangguoai"
	sourceHuangguoVideo = "huangguo-video"
	sourceHuangdou      = "huangdou"
	sourceHongguo       = "hongguo"
	sourceBeabox        = "beabox"
	sourceHeitao        = "heitao"
	sourceJinguoDuanju  = "jinguoduanju"
	sourceJinguo        = "jinguo"
)

type providerEpisode struct {
	Key   string
	Title string
	URL   string
	Index int
	HLS   string
}

func providerDramaID(source, sourceID string) string {
	return source + ":" + strings.TrimSpace(sourceID)
}

func providerChapterID(source, sourceID, chapterKey string) string {
	return source + ":" + strings.TrimSpace(sourceID) + ":" + strings.TrimSpace(chapterKey)
}

func splitProviderDramaID(id string) (source, sourceID string, ok bool) {
	source, sourceID, ok = strings.Cut(strings.TrimSpace(id), ":")
	source = canonicalProviderSource(source)
	if !ok || strings.TrimSpace(sourceID) == "" || !isHuangguoProviderSource(source) {
		return "", "", false
	}
	return source, strings.TrimSpace(sourceID), true
}

func isHuangguoProviderSource(source string) bool {
	switch canonicalProviderSource(source) {
	case sourceHuangguoAI, sourceHuangguoVideo, sourceHuangdou, sourceHongguo, sourceJinguoDuanju, sourceJinguo, sourceHeitao, sourceBeabox:
		return true
	default:
		return false
	}
}

func canonicalProviderSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "huangguo", "huangguoai", "huangguoai.com":
		return sourceHuangguoAI
	case "huangguo-video", "huangguo.video":
		return sourceHuangguoVideo
	case "huangdou", "tideember.cc", "xqjurgek.top":
		return sourceHuangdou
	case "hongguo", "hongguoduanju.com":
		return sourceHongguo
	case "jinguoduanju", "jinguoduanju.app", "www.jinguoduanju.app", "play.jinguoduanju.app", "jgdj.app", "www.jgdj.app":
		return sourceJinguoDuanju
	case "jinguo":
		return sourceJinguo
	case "heitao", "heitaodj.com", "www.heitaodj.com":
		return sourceHeitao
	case "beabox":
		return sourceBeabox
	default:
		return strings.TrimSpace(source)
	}
}
