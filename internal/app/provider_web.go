package app

// 网页系站源(禁果等)共用的校验与播放地址准备辅助,移植自 guoapp provider_web.go。

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
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

func (d *Downloader) fetchMediaPlaylist(ctx context.Context, address, referer string) (string, string, error) {
	if !isProviderHTTPMediaURL(address) {
		return "", "", errors.New("播放列表地址无效")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", "", err
	}
	request.Header.Set("Referer", referer)
	if d.limiter != nil {
		release, err := d.limiter.acquire(ctx, request)
		if err != nil {
			return "", "", err
		}
		defer release()
	}
	transport := *d.client
	transport.Jar = nil
	response, err := transport.Do(request)
	if err != nil {
		return "", "", fmt.Errorf("播放列表连接失败：%w", publicError(err))
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return "", "", errors.New("播放列表过大或读取失败")
	}
	if response.StatusCode != http.StatusOK || catalogResponseBlockReason(response, body) != "" {
		return "", "", d.catalogResponseError(request, response, body)
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(body), "\ufeff"))
	if !strings.HasPrefix(text, "#EXTM3U") {
		var envelope apiEnvelope
		if json.Unmarshal(body, &envelope) == nil && strings.Trim(string(envelope.Code), `"`) == "5005" {
			return "", "", &legacyAPIError{code: "5005", message: "播放会话已失效"}
		}
		return "", "", errors.New("站源未返回有效的播放列表，链接可能已失效")
	}
	finalURL := address
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
	}
	return text, finalURL, nil
}

func validNativeCoverURL(address *url.URL) bool {
	return address != nil && (address.Scheme == "https" || address.Scheme == "http") && address.Hostname() != "" && address.User == nil
}

func legacyCoverURL(value any) string {
	address := strings.TrimSpace(coverPathFromAny(value))
	if address == "" {
		return ""
	}
	if strings.HasPrefix(address, "//") {
		address = "https:" + address
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.User != nil {
		return ""
	}
	if parsed.IsAbs() {
		if validNativeCoverURL(parsed) {
			return address
		}
		return ""
	}
	if strings.Contains(address, "..") || strings.Contains(address, "://") {
		return ""
	}
	address = strings.TrimLeft(address, "/")
	if strings.HasPrefix(address, "upload/") || strings.HasPrefix(address, "upload_01/") {
		return "https://pic.zdmhyg.cn/" + address
	}
	return "https://zzzznnn.lkkwip.cn/" + address
}

func providerMediaOrigin(address *url.URL) string {
	if address == nil {
		return ""
	}
	scheme := strings.ToLower(address.Scheme)
	host := strings.ToLower(address.Hostname())
	if host != "" && strings.HasSuffix(host, ":80") && scheme == "http" {
		host = strings.TrimSuffix(host, ":80")
	}
	if host != "" && strings.HasSuffix(host, ":443") && scheme == "https" {
		host = strings.TrimSuffix(host, ":443")
	}
	return scheme + "://" + host
}

func dsdRouteParameters(pageURL, reference string) (string, map[string]string, bool) {
	page, err := url.Parse(pageURL)
	if err != nil || reference == "" {
		return "", nil, false
	}
	address, err := url.Parse(reference)
	if err != nil {
		return "", nil, false
	}
	address = page.ResolveReference(address)
	if address.User != nil || providerMediaOrigin(address) != providerMediaOrigin(page) {
		return "", nil, false
	}
	path := strings.TrimPrefix(address.Path, "/index.php")
	parts := strings.Split(strings.Trim(strings.TrimSuffix(path, ".html"), "/"), "/")
	if len(parts) < 2 || parts[0] != "vod" || len(parts)%2 != 0 {
		return "", nil, false
	}
	switch parts[1] {
	case "type", "show", "search", "play", "detail":
	default:
		return "", nil, false
	}
	parameters := map[string]string{}
	for index := 2; index+1 < len(parts); index += 2 {
		parameters[parts[index]] = parts[index+1]
	}
	for key, values := range address.Query() {
		if len(values) == 1 && parameters[key] == "" {
			parameters[key] = values[0]
		}
	}
	return parts[1], parameters, true
}

func writeNativeCacheFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".cache-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	_, writeErr := temporary.Write(data)
	closeErr := temporary.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary.Name(), path)
}

func firstPresent(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok && value != nil && nativeText(value) != "" {
			return value
		}
	}
	return nil
}

type providerTextUserAgentKey struct{}

type providerTextCookieKey struct{}

type providerTextNoCacheKey struct{}

func providerHTMLAttr(node *html.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return attribute.Val
		}
	}
	return ""
}

func providerHTMLClass(node *html.Node, name string) bool {
	if node == nil {
		return false
	}
	for _, class := range strings.Split(providerHTMLAttr(node, "class"), " ") {
		if strings.TrimSpace(class) == name {
			return true
		}
	}
	return false
}

func providerHTMLNodes(root *html.Node, match func(*html.Node) bool) []*html.Node {
	if root == nil {
		return nil
	}
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if match(node) {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return nodes
}

func providerHTMLFirstClass(root *html.Node, names ...string) *html.Node {
	if root == nil {
		return nil
	}
	var match func(*html.Node) bool
	match = func(node *html.Node) bool {
		for _, name := range names {
			if providerHTMLClass(node, name) {
				return true
			}
		}
		return false
	}
	nodes := providerHTMLNodes(root, match)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

func providerHTMLText(root *html.Node) string {
	if root == nil {
		return ""
	}
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return strings.TrimSpace(builder.String())
}

func (d *Downloader) fetchProviderPage(ctx context.Context, address, referer, agent string) (*html.Node, string, error) {
	responses := &playbackResponseURLs{}
	ctx = context.WithValue(ctx, playbackResponseURLsKey{}, responses)
	if agent != "" {
		ctx = context.WithValue(ctx, providerTextUserAgentKey{}, agent)
	}
	body, err := d.fetchProviderText(ctx, address, referer)
	if err != nil {
		return nil, "", err
	}
	if len(body) > 4<<20 {
		return nil, "", errors.New("站源页面过大")
	}
	if actual, found := responses.values.Load(address); found {
		address = actual.(string)
	}
	document, err := html.Parse(strings.NewReader(body))
	return document, address, err
}

func nativeDecodeCover(data []byte) []byte {
	if nativeIsCoverImage(data) {
		return data
	}
	if image, ok := nativeDecodeTextCover(data); ok {
		return image
	}
	encrypted := data
	if bytes.HasPrefix(encrypted, []byte("Salted__")) && len(encrypted) > 16 {
		encrypted = encrypted[16:]
	}
	if len(encrypted) > 0 {
		block, err := aes.NewCipher([]byte("f5d965df75336270"))
		if err == nil {
			plain := make([]byte, (len(encrypted)+aes.BlockSize-1)/aes.BlockSize*aes.BlockSize)
			copy(plain, encrypted)
			cipher.NewCBCDecrypter(block, []byte("97b60394abc2fbe1")).CryptBlocks(plain, plain)
			plain = plain[:len(encrypted)]
			if unpadded, err := pkcs7Unpad(plain, aes.BlockSize); err == nil {
				plain = unpadded
			}
			if nativeIsCoverImage(plain) {
				return plain
			}
		}
	}
	plain := append([]byte(nil), data...)
	key := []byte("2019ysapp7527")
	for index := 0; index < min(100, len(plain)); index++ {
		plain[index] ^= key[index%len(key)]
	}
	if nativeIsCoverImage(plain) {
		return plain
	}
	return data
}

func nativeDecodeTextCover(data []byte) ([]byte, bool) {
	if len(data) < 16 {
		return nil, false
	}
	for index, char := range []byte("data:") {
		if data[index]^18 != char {
			return nil, false
		}
	}
	decoded := append([]byte(nil), data...)
	for index := 0; index < min(4096, len(decoded)); index++ {
		decoded[index] ^= 18
	}
	text := string(decoded)
	if !strings.HasPrefix(text, "data:image/") {
		return nil, false
	}
	comma := strings.Index(text, ",")
	if comma < 0 || comma+1 >= len(text) {
		return nil, false
	}
	encoded := strings.TrimSpace(text[comma+1:])
	if decodedBytes, err := base64.StdEncoding.DecodeString(encoded); err == nil {
		return decodedBytes, true
	}
	if decodedBytes, err := base64.RawStdEncoding.DecodeString(encoded); err == nil {
		return decodedBytes, true
	}
	return nil, false
}

func nativeIsCoverImage(data []byte) bool {
	if isHEICImage(data) {
		return true
	}
	if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) ||
		bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) || bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		return true
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return true
	}
	if len(data) >= 16 && string(data[4:8]) == "ftyp" {
		switch string(data[8:12]) {
		case "avif", "avis", "heic", "heix", "hevc", "hevx", "mif1":
			return true
		}
	}
	return false
}

