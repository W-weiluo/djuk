package app

// 基准向量来自 guoapp-modules/hongguo main.js 的 Node 差分装置(固定随机数 0x07 填充)。

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"testing"
	"time"
)

func mustHexVector(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("向量 hex 无效: %v", err)
	}
	return decoded
}

func md5SumBytes(data []byte) []byte {
	sum := md5.Sum(data)
	return sum[:]
}

func TestHongguoV5Vectors(t *testing.T) {
	restore := hongguoRandRead
	defer func() { hongguoRandRead = restore }()
	hongguoRandRead = func(b []byte) (int, error) {
		for i := range b {
			b[i] = 7
		}
		return len(b), nil
	}

	params := url.Values{
		"aid":                 {"8662"},
		"app_name":            {"novelread"},
		"version_code":        {"74332"},
		"version_name":        {"7.4.3.32"},
		"channel":             {"vivo_8662_64"},
		"device_platform":     {"android"},
		"device_type":         {"PGT-AN10"},
		"os_version":          {"12"},
		"update_version_code": {"74332"},
		"iid":                 {"730011700000000"},
		"cdid":                {"0a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d"},
		"device_id":           {"424242424242424242"},
		"_rticket":            {"1765500000000"},
	}
	body := mustHexVector(t, "7b2262697a5f706172616d223a7b226465766963655f6c6576656c223a332c226e6565645f616c6c5f766964656f5f646566696e6974696f6e223a747275652c22766964656f5f706c6174666f726d223a313032347d2c226d697865645f766964656f5f69645f6d6170223a7b2231303034223a5b22794c6b645a78225d7d2c2264725f7363656e65223a22706c61796572227d")
	wantQuery := "_rticket=1765500000000&aid=8662&app_name=novelread&cdid=0a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d&channel=vivo_8662_64&device_id=424242424242424242&device_platform=android&device_type=PGT-AN10&iid=730011700000000&os_version=12&update_version_code=74332&version_code=74332&version_name=7.4.3.32"
	if got := params.Encode(); got != wantQuery {
		t.Fatalf("Encode 输出与向量不符:\n%s", got)
	}
	params.Set("_rticket", "1765500000009")
	queryHash := hongguoSM3([]byte(params.Encode()))
	if got := hex.EncodeToString(queryHash[:]); got != "84711b526feaba88b1beef36d800c4d4ebadc59fd7f8460f473685bf9bef1827" {
		t.Fatalf("queryHash 不符: %s", got)
	}
	bodyMd5 := md5SumBytes(body)
	if got := hex.EncodeToString(bodyMd5); got != "f19531080a7c106e4c6e8981f1cdf7ff" {
		t.Fatalf("bodyMd5 不符: %s", got)
	}
	const stamp = uint32(1765500000)
	if iv := hongguoPlaybackV5IV(queryHash[:], bodyMd5, stamp); iv != 0x5c366e1c {
		t.Fatalf("IV 不符: %x", iv)
	}
	f13, err := hongguoPlaybackV5Hash(queryHash[:], bodyMd5, stamp)
	if err != nil {
		t.Fatalf("V5 hash 报错: %v", err)
	}
	if got := hex.EncodeToString(f13); got != "695c187c9250951b54838b24e2a06cb9369a42a4" {
		t.Fatalf("f13 不符: %s", got)
	}
	message := hongguoProtoFields(
		uint64(1), hongguoPlaybackV5Magic, uint64(2), uint64(5), uint64(3), uint64(hongguoU32LE([]byte{7, 7, 7, 7}, 0)), uint64(4), "8662",
		uint64(5), "424242424242424242", uint64(6), "646637794", uint64(7), "7.4.3.32", uint64(8), "v04.10.03-ml-android",
		uint64(9), uint64(0x040a0300), uint64(10), make([]byte, 8), uint64(12), uint64(stamp), uint64(13), f13,
		uint64(14), queryHash[:6], uint64(15), hongguoProtoFields(uint64(1), uint64(1), uint64(2), uint64(1), uint64(3), uint64(1), uint64(5), uint64(1)),
		uint64(17), uint64(stamp), uint64(20), "none", uint64(21), uint64(377), uint64(23), hongguoProtoFields(uint64(6), "v04.10.03.04-bugfix", uint64(8), "vivo_8662_64"),
		uint64(24), `{"cmr":16777216,"cmr2":16777216,"un_h":11,"vpn":0,"kd":0,"dyn":"","do":1,"tk":true}`,
	)
	wantMessage := "0a102d4b4fca49750d433fb5ae2c226dcc56100a188e9cb8702204383636322a1234323432343234323432343234323432343232093634363633373739343a08372e342e332e333242147630342e31302e30332d6d6c2d616e64726f696448808cd0405208000000000000000060c091db930d6a14695c187c9250951b54838b24e2a06cb9369a42a4720684711b526fea7a0808021002180228028801c091db930da201046e6f6e65a801f205ba012332137630342e31302e30332e30342d627567666978420c7669766f5f383636325f3634c201537b22636d72223a31363737373231362c22636d7232223a31363737373231362c22756e5f68223a31312c2276706e223a302c226b64223a302c2264796e223a22222c22646f223a312c22746b223a747275657d"
	if got := hex.EncodeToString(message); got != wantMessage {
		t.Fatalf("message 不符: %s", got)
	}
	vectorKey := mustHexBytes("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f2021222324252627")
	xmxored := hongguoPlaybackV5Xmxor(message, vectorKey)
	wantXmxor := "a8751989df91c27b1f555db3477b2b43f637ba1eb559d4025ce6ab4e4ede92a69019747a94028c28328ce3af61763e3d568bdfd6b4483f0372c19b4166b022c99034a4ca5b392b622148b8a11ac3d5da6437f50d9ee3290c94f668fb27bf39405fefaa6635d15c9dd88ed78d17c52823dc3ae0cf2683d51dca2ea6f324f8d909a711d9219476364ce33a77751772fd009ab542ca678e853e8f6bc16641bfe748535d99b75374705ff6eb25ef6e089ee42bee268ff24e566c56d05df5986a812ddedcfcee9be0bcdcbaeffb751f263f605104fa7cf06e2a265ac1d189cfe4eb9478314d650e2d7feca1eac7a92d5c60d7e3b04cc86db1e9ff21d9ca747aae0ee7ded8c5f3b18d245d397e1f5c20ba0d32a26dda85007b3174e203377b6d3cf323c235428864dee8404c"
	if got := hex.EncodeToString(xmxored); got != wantXmxor {
		t.Fatalf("xmxor 不符: %s", got)
	}
	check := uint32(0x18000001) | (uint32(queryHash[0]&63) << 14) | (uint32(f13[0]&63) << 8)
	packed := append([]byte{0xca}, []byte{7, 7, 7, 7}...)
	packed = append(packed, hongguoLe32(check)...)
	packed = append(packed, xmxored...)
	packed = append(packed, 0, 0)
	md5Low := md5SumBytes(hongguoPlaybackV5SignKey[:16])
	md5High := md5SumBytes(hongguoPlaybackV5SignKey[16:])
	carrier, err := hongguoPlaybackV5Carrier(md5Low, md5High, packed, stamp)
	if err != nil {
		t.Fatalf("carrier 报错: %v", err)
	}
	wantCarrier := "668a270f1703812b0158887d198ddf93c37b3f5d5db7477b2b43d637aa1e3559d4025ceeab4a4ede92a6b011647e14028d68128ce3abe1763f3d5683dfd2b44a3f0372c98b4166b223899034b4ce5b3b2b220148a8a59ac3d4da443fe5099ee1290c94fe68fba7bd38007fe7aa6635d35cddf88ec78917c52823dc32f0cf2683d41dca2ea6f7a4fad9098711d9211476360ce33267711772fc009abd52ce678c853eaf6bd166c1bde708735d99b75374715fd6eb35ef6e0a9fe40bee368b724e562c76d84df51868816ddedcfcea1be2bc9cbae7eb751f243f605104fa78f06c2b267ac1d18dcfe4ea9458395d658e2d7eaca1e2c7a92d5e61d7e3b04cc86db1e9ff21d9ca747aae0ee7ded8c5f3b18d245d397e1f5c20ba0d32a26dda85007b3174e203377b6d3cf323c235428864dee8404c0000"
	if got := hex.EncodeToString(carrier); got != wantCarrier {
		t.Fatalf("carrier 不符: %s", got)
	}
	if helios := hongguoPlaybackV5Helios(stamp); helios != "BwcHB7Z3C/68PjYaJoo6j8pK0ePGr82H7PNRXkeHFOOdndko" {
		t.Fatalf("helios 不符: %s", helios)
	}

	// 端到端:signPlaybackV5(固定随机数)
	downloader := &Downloader{}
	signedURL, headers, err := downloader.hongguoSignPlaybackV5("https://api5-normal-sinfonlinea.fqnovel.com", params, body, time.UnixMilli(1765500000000))
	if err != nil {
		t.Fatalf("v5 签名报错: %v", err)
	}
	wantURL := "https://api5-normal-sinfonlinea.fqnovel.com?_rticket=1765500000009&aid=8662&app_name=novelread&cdid=0a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d&channel=vivo_8662_64&device_id=424242424242424242&device_platform=android&device_type=PGT-AN10&iid=730011700000000&os_version=12&update_version_code=74332&version_code=74332&version_name=7.4.3.32"
	if signedURL != wantURL {
		t.Fatalf("v5 URL 不符: %s", signedURL)
	}
	if headers["X-Khronos"] != "1765500000" || headers["X-Helios"] != "BwcHB7Z3C/68PjYaJoo6j8pK0ePGr82H7PNRXkeHFOOdndko" {
		t.Fatalf("helios/khronos 不符: %v", headers)
	}
	wantMedusa := "ZWQ7aU0vdKMpETYqX9GVRUIJ9z8HBwABz4onBxcDgSsAWH//N8byOsHaJaypcZBlL+Eu8ATJMrJhp9SmpuDbc1yvw8d6Vu2+uqDjmmg9LJvLdP0XyrPS05ITtbot5xSgvvZSGjimK2XGYQL3GukavCOvfqYGMI3cliWVtlygjB+DwuEgEa9Oza61lQ55022pAJrIibw+Oq2DBvtPBKUJP0qGsRLzZUUy4kHQmSEzSzIo13zvQ+a2Pp3zwOLAnmGSvlBKQhCn48aqpcN8EWsP0m3P7HGAUoRWldPdrnwcrNZ/vP6Eei53nyTzqm9DqumUfN4Y4SC8hhnUaqKxSNPQx2dnzC2aQi3m6di8OTJKnUfdGK/h3xkaPIC2HfvSO+/EmarmID0h0g/cNa2PccqxNzQCRKVLGCh8cfx1UGhUkjDvDs0CZ3fZB53ejbO2Ji6FfVeifnQhGKnudtXdFNQagYK7qIXG04ziBpuvTRXXplCSoApunm5Y9f1sgl8/K5OsJXeSXRZklVjUl6GDT2wsR36IRW90FrF8T3T7ImWYd8pB4oBhC7ZuG5WT6rsoa89KnfRsZE/5D6M2VzthX+SMQHcWmr/81FgeVEct8RZrcO2Ag18uuSSsOX04LwAIhBlHQckYbN9L///H////x/8HBw=="
	if headers["X-Medusa"] != wantMedusa {
		t.Fatalf("medusa 不符: %s", headers["X-Medusa"])
	}
	if headers["X-SS-STUB"] != "F19531080A7C106E4C6E8981F1CDF7FF" {
		t.Fatalf("stub 不符: %s", headers["X-SS-STUB"])
	}
}
