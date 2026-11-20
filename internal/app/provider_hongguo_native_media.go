package app

// 红果取流:双 API 链(videoTab 主 + multi_video_model 备用)与 H.265 备用线路,
// 移植自 guoapp-modules/hongguo v1.0.7 main.js(GPL-3.0-only)。

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// errHongguoNoCompatibleMedia 标记"没有可用兼容线路",触发备用 API 回退。
var errHongguoNoCompatibleMedia = errors.New("红果 App 未返回兼容的媒体")

var hongguoPlaybackPlayURLSalts = []string{
	"4dd4c2e6b83162090e52b3c7a6733ba41cb2462b829ab58a196b39db57177524f49baf7f08e8d68d26a72e37c1a95a2f1f05a51892aef2949732b62a38aadd58",
}

const (
	hongguoPlaybackURLHeaderSize = 4
	hongguoPlaybackFallback1     = "1"
	hongguoPlaybackFallback3     = "3"
)

var (
	hongguoPlaybackURLPattern   = regexp.MustCompile(`^https?://\S+$`)
	hongguoPlaybackCodecReplace = regexp.MustCompile(`([?&])codec_type=[^&]*`)
)

func hongguoHelperContext(ctx context.Context, helper bool) context.Context {
	return context.WithValue(ctx, hongguoHelperDeviceKey{}, helper)
}

func (downloader *Downloader) resolveHongguoAppMedia(ctx context.Context, videoID string) (providerMedia, error) {
	if !hongguoNumericID.MatchString(videoID) {
		return providerMedia{}, errors.New("红果视频 ID 无效")
	}
	var lastErr error
	for _, helper := range []bool{false, true} {
		media, err := downloader.fetchHongguoAppMedia(hongguoHelperContext(ctx, helper), videoID, helper)
		if err == nil {
			return media, nil
		}
		if errors.Is(err, errHongguoNoCompatibleMedia) {
			return providerMedia{}, err
		}
		lastErr = err
	}
	var rejected string
	if client := downloader.hongguoClient(); client != nil {
		client.mu.Lock()
		rejected, _, _ = client.deviceLocked(true)
		client.mu.Unlock()
	}
	if err := downloader.ensureHongguoDevice(ctx, true, rejected); err == nil {
		media, err := downloader.fetchHongguoAppMedia(hongguoHelperContext(ctx, true), videoID, true)
		if err == nil {
			return media, nil
		}
	}
	return providerMedia{}, lastErr
}

func (downloader *Downloader) fetchHongguoAppMedia(ctx context.Context, videoID string, helper bool) (providerMedia, error) {
	media, tabErr := downloader.fetchHongguoTabMedia(ctx, videoID, helper)
	if tabErr == nil {
		return media, nil
	}
	if errors.Is(tabErr, errHongguoNoCompatibleMedia) {
		return providerMedia{}, tabErr
	}
	media, playerErr := downloader.fetchHongguoPlayerMedia(ctx, videoID, helper)
	if playerErr == nil {
		return media, nil
	}
	if errors.Is(playerErr, errHongguoNoCompatibleMedia) {
		return providerMedia{}, playerErr
	}
	return providerMedia{}, fmt.Errorf("%v；播放器接口：%v", tabErr, playerErr)
}

func (downloader *Downloader) fetchHongguoTabMedia(ctx context.Context, videoID string, helper bool) (providerMedia, error) {
	extra := url.Values{
		"video_id":                   {videoID},
		"video_id_type":              {"0"},
		"device_level":               {"1"},
		"need_all_video_definition":  {"1"},
		"need_mp4_align":             {"0"},
		"use_os_player":              {"0"},
		"video_platform":             {"1024"},
		"source":                     {"0"},
		"from_video_id":              {""},
	}
	result, err := downloader.hongguoAppRequest(ctx, http.MethodGet, hongguoVideoTabPath, extra, nil)
	if err != nil {
		return providerMedia{}, err
	}
	root := nestedMap(result, "data")
	data := nestedMap(nestedMap(root, "video_model_data"), videoID)
	if len(data) == 0 {
		return providerMedia{}, errors.New("红果 App 未返回所请求的分集播放信息")
	}
	return downloader.hongguoMediaFromModel(hongguoAppVideoModel(data))
}

func (downloader *Downloader) fetchHongguoPlayerMedia(ctx context.Context, videoID string, helper bool) (providerMedia, error) {
	payload := map[string]any{
		"biz_param": map[string]any{
			"detail_page_version": 0, "device_level": 1, "disable_digg_stat": false, "disable_video_relate_book": false,
			"need_all_video_definition": true, "need_mp4_align": false, "use_os_player": false, "use_server_dns": false, "video_platform": 1024,
		},
		"mixed_video_id_map": map[string]any{"1004": []string{videoID}},
		"dr_scene":           "player",
	}
	result, err := downloader.hongguoAppRequest(ctx, http.MethodPost, hongguoPlaybackPath, nil, payload)
	if err != nil {
		return providerMedia{}, err
	}
	entries := nestedMap(result, "data")
	data := entries
	if row, ok := entries["1004"].(map[string]any); ok {
		data = row
	}
	if inner := nestedMap(data, videoID); len(inner) > 0 {
		data = inner
	}
	if len(data) == 0 {
		return providerMedia{}, errors.New("红果 App 未返回所请求的分集播放信息")
	}
	return downloader.hongguoMediaFromModel(hongguoAppVideoModel(data))
}

func hongguoAppVideoModel(data map[string]any) map[string]any {
	model, _ := data["video_model"].(map[string]any)
	if encoded, ok := data["video_model"].(string); ok && encoded != "" {
		if decoded, err := decodeHongguoBase64(encoded); err == nil && len(decoded) > 0 {
			encoded = string(decoded)
		}
		decoder := json.NewDecoder(strings.NewReader(encoded))
		decoder.UseNumber()
		var parsed map[string]any
		if err := decoder.Decode(&parsed); err != nil {
			parsed = nil
		}
		if len(parsed) == 0 && model == nil {
			model = nil
		} else if len(parsed) > 0 {
			model = parsed
		}
	}
	if len(model) == 0 {
		if rows := anyList(data["video_list"]); len(rows) > 0 {
			model = map[string]any{
				"video_list":     data["video_list"],
				"video_duration": data["video_duration"],
				"duration":       data["duration"],
			}
		}
	}
	if model == nil {
		model = map[string]any{}
	}
	return model
}

func (downloader *Downloader) hongguoMediaFromModel(model map[string]any) (providerMedia, error) {
	media, err := selectHongguoAppMedia(model)
	if err == nil {
		return media, nil
	}
	if !errors.Is(err, errHongguoNoCompatibleMedia) {
		return providerMedia{}, err
	}
	return downloader.fallbackMedia(model, err)
}

func (downloader *Downloader) fallbackMedia(model map[string]any, original error) (providerMedia, error) {
	endpoint := mapString(nestedMap(nestedMap(model, "fallback_api"), "fallback_api"), "fallback_api")
	if endpoint == "" {
		return providerMedia{}, original
	}
	var lastErr error
	for _, codecType := range []string{hongguoPlaybackFallback1, hongguoPlaybackFallback3} {
		data, err := downloader.fetchHongguoFallbackPlay(endpoint, codecType)
		if err == nil {
			media, err := selectHongguoAppMedia(map[string]any{
				"video_list":     hongguoFallbackVariants(data),
				"video_duration": mapString(data, "video_duration"),
			})
			if err == nil {
				return media, nil
			}
			if !errors.Is(err, errHongguoNoCompatibleMedia) {
				return providerMedia{}, fmt.Errorf("%v；备用线路：%v", original, err)
			}
			lastErr = err
			continue
		}
		if !errors.Is(err, errHongguoNoCompatibleMedia) {
			return providerMedia{}, fmt.Errorf("%v；备用线路：%v", original, err)
		}
		lastErr = err
	}
	return providerMedia{}, fmt.Errorf("%v；备用线路：%v", original, lastErr)
}

func (downloader *Downloader) fetchHongguoFallbackPlay(endpoint, codecType string) (map[string]any, error) {
	address := endpoint
	if hongguoPlaybackCodecReplace.MatchString(endpoint) {
		address = hongguoPlaybackCodecReplace.ReplaceAllString(endpoint, "${1}codec_type="+codecType)
	} else if strings.Contains(endpoint, "?") {
		address = endpoint + "&codec_type=" + codecType
	} else {
		address = endpoint + "?codec_type=" + codecType
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("红果备用线路请求失败：%v", err)
	}
	request.Header.Set("User-Agent", hongguoPlaybackUserAgent)
	request.Header.Set("Accept", "application/json")
	response, err := downloader.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("红果备用线路请求失败：%v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("红果备用线路 HTTP %d", response.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, providerMaxBodyBytes+1)).Decode(&result); err != nil {
		result = nil
	}
	data := nestedMap(nestedMap(result, "video_info"), "data")
	if len(data) == 0 {
		return nil, errors.New("红果备用线路未返回有效数据")
	}
	return data, nil
}

func hongguoFallbackVariants(data map[string]any) []map[string]any {
	seed := mapString(data, "key_seed")
	variants := []map[string]any{}
	versionErr := error(nil)
	decoded := 0
	entries := anyList(data["video_list"])
	if len(entries) == 0 {
		if rows, ok := data["video_list"].(map[string]any); ok {
			keys := make([]string, 0, len(rows))
			for key := range rows {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				entries = append(entries, rows[key])
			}
		}
	}
	for _, value := range entries {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		variant := map[string]any{
			"gear_des_key": mapString(entry, "gear_des_key"),
			"video_meta": map[string]any{
				"codec_type": mapString(entry, "codec_type"),
				"definition": mapString(entry, "definition"),
				"vwidth":     mapString(entry, "vwidth"),
				"vheight":    mapString(entry, "vheight"),
			},
		}
		versionFailure := error(nil)
		for _, key := range []string{"main_url", "backup_url_1", "backup_url_2"} {
			address, err := hongguoDecodePlayURL(mapString(entry, key), seed)
			if err != nil {
				if !errors.Is(err, errHongguoNoCompatibleMedia) {
					versionErr = err
				}
				continue
			}
			if address != "" {
				variant[key] = address
				decoded++
			}
		}
		if versionFailure != nil {
			versionErr = versionFailure
		}
		spade := mapString(entry, "spade_a")
		method := mapString(entry, "encryption_method")
		encrypt, _ := entry["encrypt"].(bool)
		if spade != "" || method != "" || encrypt {
			variant["encrypt_info"] = map[string]any{"spade_a": spade, "encryption_method": method, "encrypt": encrypt}
		}
		variants = append(variants, variant)
	}
	if versionErr != nil && decoded == 0 {
		return nil
	}
	return variants
}

func hongguoDecodePlayURL(encoded, seed string) (string, error) {
	if encoded == "" || seed == "" {
		return encoded, nil
	}
	key, err := decodeHongguoBase64(seed)
	if err != nil {
		return encoded, nil
	}
	raw, err := decodeHongguoBase64(encoded)
	if err != nil {
		return encoded, nil
	}
	if len(raw) < 20 || raw[0] != 0xa8 || raw[1] != 0x00 || (len(raw)-hongguoPlaybackURLHeaderSize)%16 != 0 {
		return encoded, nil
	}
	for _, salt := range hongguoPlaybackPlayURLSalts {
		address := hongguoDecryptPlayURL(raw, key, salt)
		if address != "" {
			return address, nil
		}
	}
	version := int(raw[2]) | int(raw[3])<<8
	return "", fmt.Errorf("%w：红果备用线路编码版本 %d 暂不支持", errHongguoNoCompatibleMedia, version)
}

func hongguoDecryptPlayURL(raw []byte, seed []byte, salt string) string {
	seedDigest := sha512.Sum512(seed)
	digestSource := append(seedDigest[:], []byte(salt)...)
	digest := sha512.Sum512(digestSource)
	block, err := aes.NewCipher(digest[:16])
	if err != nil {
		return ""
	}
	encrypted := raw[hongguoPlaybackURLHeaderSize:]
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, digest[16:32]).CryptBlocks(plain, encrypted)
	pad := int(plain[len(plain)-1])
	if pad < 1 || pad > 16 || pad > len(plain) {
		return ""
	}
	for index := len(plain) - pad; index < len(plain); index++ {
		if plain[index] != byte(pad) {
			return ""
		}
	}
	address := string(plain[:len(plain)-pad])
	if !hongguoPlaybackURLPattern.MatchString(address) {
		return ""
	}
	return address
}

func selectHongguoAppMedia(model map[string]any) (providerMedia, error) {
	variants := anyList(model["video_list"])
	if rows, ok := model["video_list"].(map[string]any); ok && len(variants) == 0 {
		keys := make([]string, 0, len(rows))
		for key := range rows {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			variants = append(variants, rows[key])
		}
	}
	duration, _ := strconv.ParseFloat(mapString(model, "video_duration", "duration"), 64)
	var keyErr error
	type scoredMedia struct {
		media providerMedia
		score int
	}
	var choices []scoredMedia
	for _, row := range variants {
		variant, _ := row.(map[string]any)
		meta := nestedMap(variant, "video_meta")
		codec := strings.ToLower(mapString(meta, "codec_type"))
		if codec == "bytevc2" || strings.Contains(strings.ToLower(mapString(variant, "gear_des_key")), "bytevc2") {
			continue
		}
		addresses := hongguoMediaAddresses(variant)
		if len(addresses) == 0 {
			continue
		}
		media := providerMedia{Referer: "https://novel.snssdk.com/", Duration: time.Duration(duration * float64(time.Second))}
		encryption := nestedMap(variant, "encrypt_info")
		spade := mapString(encryption, "spade_a")
		if spade != "" || encryption["encrypt"] == true || mapString(encryption, "encryption_method") == "cenc-aes-ctr" {
			var err error
			media.CENCKey, err = hongguoContentKey(spade)
			if err != nil {
				keyErr = err
				continue
			}
		}
		height, _ := strconv.Atoi(mapString(meta, "vheight"))
		if definition, err := strconv.Atoi(hongguoQualityNumber.FindString(mapString(meta, "definition"))); err == nil && definition > 0 {
			height = definition
		} else if width, _ := strconv.Atoi(mapString(meta, "vwidth")); width > 0 && (height == 0 || width < height) {
			height = width
		}
		media.Quality = height
		quality := height * 10
		if codec == "h264" || codec == "avc1" {
			quality++
		}
		for _, address := range addresses {
			media.URL = address
			choices = append(choices, scoredMedia{media: media, score: quality})
		}
	}
	if len(choices) > 0 {
		sort.SliceStable(choices, func(i, j int) bool { return choices[i].score > choices[j].score })
		selected := choices[0].media
		for _, choice := range choices {
			selected.Variants = append(selected.Variants, choice.media)
		}
		return selected, nil
	}
	if keyErr != nil {
		return providerMedia{}, fmt.Errorf("红果 App 媒体密钥不可用: %w", keyErr)
	}
	return providerMedia{}, fmt.Errorf("%w，已跳过不支持的编码", errHongguoNoCompatibleMedia)
}

func hongguoMediaAddresses(info map[string]any) []string {
	var addresses []string
	seen := map[string]bool{}
	var add func(any)
	add = func(value any) {
		switch value := value.(type) {
		case string:
			address := strings.TrimSpace(value)
			if len(address) > 8192 {
				return
			}
			if !isProviderHTTPMediaURL(address) {
				decoded, err := decodeHongguoBase64(address)
				if err != nil {
					return
				}
				address = strings.TrimSpace(string(decoded))
			}
			if isProviderHTTPMediaURL(address) && !seen[address] {
				seen[address] = true
				addresses = append(addresses, address)
			}
		case []any:
			for _, item := range value {
				add(item)
			}
		}
	}
	for _, key := range []string{"main_url", "backup_url", "backup_url_1", "backup_url_2", "backup_urls", "url_list"} {
		add(info[key])
	}
	return addresses
}
