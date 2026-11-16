package app

// 红果取流 v5 签名,移植自 guoapp-modules/hongguo main.js(signPlaybackV5 家族)。
// 上游注明派生来源 N3urda/hongguoTV Signer/AesV3/V5Constants 与 drpys 22261ad,
// GPL-3.0-only。11-16 网关切换到 multi_video_model/player/v1 + v5 签名(#165)。

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/bits"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	hongguoPlaybackBaseURL   = "https://api5-normal-sinfonlinea.fqnovel.com"
	hongguoPlaybackPath      = "/novel/player/multi_video_model/player/v1"
	hongguoPlaybackUserAgent = "com.phoenix.read/73932 (Linux; U; Android 12; zh_CN_#Hans; PGT-AN10; Build/V417IR;tt-ok/3.12.13.20)"
)

var hongguoRandRead = rand.Read

type hongguoPlaybackV5Process struct {
	nonce   string
	started int64
}

var (
	hongguoPlaybackV5Magic   = mustHexBytes("2d4b4fca49750d433fb5ae2c226dcc56")
	hongguoPlaybackV5SignKey = mustHexBytes("b23c2a470a20d105dc3f8c4438f8272ed46d6c89ffdcdedd5108698d26e9489d")
	hongguoPlaybackV5Sbox    = mustHexBytes(
		"53290fe9e51f316f90f74a7e034d367784ab49237cd0135cdc632c322fd3be0e2624a417ecd9151a7ff64f60730a87f9445ec6ada2f865d22b696c4188f24c0d" +
			"f37b75510b939f6bc2fe6883e68bc8c722181ba0bdb08c9871b50c9a16b46eea8eed20ba4b8a14e0fcf1468dbbef05a93e5a9d7dcf06898635c1fd99f5de287a" +
			"80c594daaab19b6279fbd75070cb52eb2555c456e39e7839853874e2f0013ac0b64742d8ca10e4b79c955de8b843ce4ed464a6a72ed508091c92e7b3eedd1e1d" +
			"d12afac321618fa3d65f9167c982f43d66a53c6a1997585b346dbf9672ff37cdbc073b2dae11453304df00815476b2a13fccdb30b957a8acafe1274002485912" +
			"38922563e36406d37a24100b7902a028044e218453c2a399199c9d13dc40c55ba67e502273850ebdc1a2e2188b8f2d9e3cb909b8c46a2a9f4a6dd93007eafa65" +
			"b7b3a881aa908cc74f0ad76bbbb58617c37012ca05a469fd44e62eecf1e4272ce860c0ae5eadf99b20d8f0234174006f3f110d7657d01645d4eff8f71471cbfe" +
			"1cb2f52648727bf3c9de55153e031b4d61757db63dbf7782eeafe9018a8891362fb14759ac32bab03b349aeb5a377c5156be4c8dfbe13a684bda39cf836697c8" +
			"94545fe0f2491acea9fc6252f4ff1e0fe72bd2df6e67a7f635e5bcc6d1a5dd951d89960887a18eb40c785cdb1f80ed42299333ab98cdd67f316cd54346585dcc" +
			"d759fb722789b08807cc0fda293f7e5e1819bb0353517020b31d3d00374952bea2c421e862040a9ec892a05b266b7a0d0cc316f483b81af39715657ffe32ad8d" +
			"e0c13077646035236ee5363e5f900e9a7cf850a81cd9960b6df73317226f99eaa6fff15cec34acb98246639b918ab2851466a7cdfc4ad1b52c2ae95a58d3734e" +
			"47ddd5b7560193f644ef9c408b7dee39b4d6aabf4de761e3252d4c7ba3843187745d55e6a5db05d42b45caed43c0c5d0b64bfa9d94a4a9d8f2f0e1a108dcfd28" +
			"8fd2bc1fc7bd675412138169cb09249875ba3b68cfb11180afae104fc9789571c6dfce02413886796ae21e3c488cc21b9f423a066c2eabf98e2fdeeb5776e4f5" +
			"d09a515ba58cbbab3e37b09fd4f6d9244df80512e84cd7956be301cf64f1429d58577cc241f3a2c655144f81aaf95a4e108da894b38970561ae9002619fbbac9" +
			"1fa922ed202bfc62cb08e0de2a7f3631435f8b47c8a3c563b4db4071ce74c46da6fa696a9e282d1e825d663b93bfd56c48dce1884bb10fccea063929f746443f" +
			"d830a4232e53da21a1f2d1bdfebc3a02c10a1b8ad31d04fd616590d272ac737e98eb33af60528716bee78ec0ee8699031177277d9cff32542c67caf5f4459168" +
			"1335ae0e178f3409a7803db83892dde5b9b7f0c750e24aef077badd678a09b850b84ec1597e4963cb5e675df1c0d6fb618496ecd76c35c798325590c2fb27a5e")
	hongguoPlaybackV5HashTable = [64]uint32{
		0xebb64faf, 0x07aadcc2, 0xcf3187bf, 0xe01138ff, 0x6d0bfcff, 0x5a30a3be, 0xb41ad638, 0x34180eb8,
		0xf233eb6f, 0xb1a584cc, 0xccc30dc7, 0x47d1db51, 0xd55653de, 0x70a84fa1, 0x57473c12, 0xf76f0288,
		0x2c077f0a, 0xda0dcad0, 0xfbb86f6c, 0xfdc4cf00, 0x688a020d, 0xe676c6a6, 0x8cd6338b, 0x1a3c8d0e,
		0xcce8b06b, 0x6ad0ed0b, 0xa0522717, 0xdc71ac83, 0x2285db71, 0xd5b4dda6, 0x736f8650, 0x6560306c,
		0x617ce2a6, 0xe423417e, 0x0a40e143, 0x544e4032, 0x88dffb2a, 0x716c1ae0, 0x4c467a88, 0x05b23bb3,
		0xe1d0b866, 0xbaa3dcb8, 0xae3374d3, 0xc3381a50, 0x1702f75b, 0xfe6da368, 0xf0b4cf48, 0x4e0ffbb8,
		0x72aad10d, 0x26c53a3d, 0xf2bce0f6, 0xb4557581, 0x4a257fdd, 0x8c3182a2, 0xab0b3b86, 0x3d5dfb14,
		0x4f103634, 0xd37b52d7, 0x444eff16, 0xeb0a33d1, 0x6ca86f6e, 0x00284ba7, 0x08387cfa, 0x5fb37586,
	}
)

func hongguoRotl8(value byte, shift int) byte {
	return bits.RotateLeft8(value, shift)
}

func hongguoRotl64(value uint64, shift int) uint64 {
	return bits.RotateLeft64(value, shift)
}

func hongguoWriteU64(data []byte, offset int, value uint64) {
	binary.LittleEndian.PutUint64(data[offset:], value)
}

func mustHexBytes(value string) []byte {
	decoded, err := hex.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return decoded
}

func hongguoLe32(value uint32) []byte {
	return []byte{byte(value), byte(value >> 8), byte(value >> 16), byte(value >> 24)}
}

func hongguoU32LE(data []byte, offset int) uint32 {
	return uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
}

func hongguoReadU64(data []byte, offset int) uint64 {
	return uint64(hongguoU32LE(data, offset)) | uint64(hongguoU32LE(data, offset+4))<<32
}

func hongguoPad16(data []byte) []byte {
	padding := 16 - len(data)%16
	out := make([]byte, len(data)+padding)
	copy(out, data)
	for index := len(data); index < len(out); index++ {
		out[index] = byte(padding)
	}
	return out
}

func hongguoProtoVarint(out []byte, field uint64, value uint64) []byte {
	out = appendUvarintBytes(out, field*8)
	return appendUvarintBytes(out, value)
}

func hongguoProtoBytes(out []byte, field uint64, value []byte) []byte {
	out = appendUvarintBytes(out, field*8+2)
	out = appendUvarintBytes(out, uint64(len(value)))
	return append(out, value...)
}

func appendUvarintBytes(out []byte, value uint64) []byte {
	for value >= 128 {
		out = append(out, byte(value%128)|128)
		value /= 128
	}
	return append(out, byte(value))
}

func hongguoProtoFields(fields ...any) []byte {
	var out []byte
	for index := 0; index+1 < len(fields); index += 2 {
		tag, _ := fields[index].(uint64)
		switch value := fields[index+1].(type) {
		case uint64:
			out = hongguoProtoVarint(out, tag, value*2)
		case string:
			if value != "" {
				out = hongguoProtoBytes(out, tag, []byte(value))
			}
		case []byte:
			if len(value) != 0 {
				out = hongguoProtoBytes(out, tag, value)
			}
		}
	}
	return out
}

func hongguoPlaybackV5IV(queryHash, bodyMd5 []byte, stamp uint32) uint32 {
	value := uint32(0x20230928)
	for _, data := range [][]byte{queryHash, bodyMd5, hongguoLe32(stamp)} {
		for index, element := range data {
			if index%2 == 0 {
				value = (value >> 4) ^ value ^ (value << 6) ^ uint32(element)
			} else {
				value = ^((value >> 7) ^ value ^ (uint32(element) | (value << 12)))
			}
		}
	}
	return value
}

func hongguoPlaybackV5Hash(queryHash, bodyMd5 []byte, stamp uint32) ([]byte, error) {
	if len(queryHash) != 32 || len(bodyMd5) != 16 {
		return nil, errors.New("V5 hash 需要 32 字节 query 哈希与 16 字节 body 哈希")
	}
	if hongguoPlaybackV5IV(queryHash, bodyMd5, stamp)&15 != 12 {
		return nil, errors.New("不支持的 V5 hash 分支")
	}
	var ivValue uint32 = 0x9d4a97c2
	iv := int32(ivValue)
	count := iv + int32(stamp)
	table := make([]uint32, len(hongguoPlaybackV5HashTable))
	for index, value := range hongguoPlaybackV5HashTable {
		table[index] = bits.RotateLeft32(value, -int(int32(count+1)&255))
	}
	shift := uint(int32(count+2) & 7)
	padding := []byte{0xfa, 0x45, 0x61, 0xd7}
	for index, value := range padding {
		padding[index] = byte(((uint32(value) | (uint32(value) << 8)) >> shift) & 255)
	}
	data := append([]byte{}, queryHash...)
	data = append(data, bodyMd5...)
	data = append(data, hongguoLe32(stamp)...)
	data = append(data, padding...)
	data = append(data, 0, 0, 0, 0, 0, 0, 1, 0xa0)
	words := make([]uint32, 0, len(data)/4+112)
	for index := 0; index+3 < len(data); index += 4 {
		words = append(words, uint32(data[index])<<24|uint32(data[index+1])<<16|uint32(data[index+2])<<8|uint32(data[index+3]))
	}
	for index := 0; index < 112; index++ {
		x, y := hongguoWordAt(words, index+1), hongguoWordAt(words, index+14)
		words = append(words, hongguoWordAt(words, index)+hongguoWordAt(words, index+9)+
			(bits.RotateLeft32(x, 14)^bits.RotateLeft32(x, 25)^(x>>3))+
			(bits.RotateLeft32(y, 13)^bits.RotateLeft32(y, 15)^(y>>10)))
	}
	initial := []uint32{0x7aba4fc8, 0x67166507, 0x6403fa00, 0x340f512f, 984304912, 3005047866, 2874125293, 2152413264}
	for index, value := range initial {
		initial[index] = bits.RotateLeft32(value, int((-count)&31))
	}
	state := make([]uint32, 8)
	copy(state, initial)
	for index := 0; index < 96; index++ {
		base := iv + int32(index)
		choose := (state[4]^state[0])&state[1] ^ state[4]
		sigma1 := bits.RotateLeft32(state[1], 26) ^ bits.RotateLeft32(state[1], 21) ^ bits.RotateLeft32(state[1], 7)
		mixed := words[int(base)&127] + choose + sigma1 + table[int(base)&63] + state[6]
		sigma0 := bits.RotateLeft32(state[3], 30) ^ bits.RotateLeft32(state[3], 19) ^ bits.RotateLeft32(state[3], 10)
		majority := state[3]&state[5] | (state[3]|state[5])&state[2]
		old := state[7]
		copy(state[1:], state[:7])
		state[0] = old
		state[7] = sigma0 + majority + mixed
		state[0] = old + mixed
	}
	digest := make([]byte, 32)
	for index := 0; index < 8; index++ {
		binary.BigEndian.PutUint32(digest[index*4:], state[index]+initial[index])
	}
	folded := make([]byte, 16)
	for index := 0; index < 16; index++ {
		folded[index] = digest[index] ^ digest[index+16]
	}
	check := uint32(0x20220420)
	for index := 0; index < 12; index++ {
		var temp uint32
		if index%2 == 0 {
			temp = (check >> 3) ^ check
		} else {
			temp = (check >> 5) ^ check
		}
		if index%2 == 0 {
			check = uint32(folded[index]) ^ (check << 7) ^ temp
		} else {
			check = ^(uint32(folded[index]) | (check << 11)) ^ temp
		}
	}
	result := append(folded, hongguoLe32((check|4)^0x1000000|0x20)...)
	return result, nil
}

func hongguoPlaybackV5Carrier(keyBytes, ivBytes, sourceBytes []byte, stamp uint32) ([]byte, error) {
	if len(keyBytes) != 16 || len(ivBytes) != 16 || len(sourceBytes) < 248 {
		return nil, errors.New("V5 carrier 需要 16 字节密钥/IV 与至少 248 字节源数据")
	}
	variant := int(stamp & 3)
	box := hongguoPlaybackV5Sbox[variant*256 : (variant+1)*256]
	con := [4][4]int{{2, 0, 3, 1}, {1, 3, 2, 0}, {3, 2, 1, 0}, {2, 3, 1, 0}}[variant]
	order := [4][16]int{
		{0, 9, 14, 15, 4, 13, 2, 7, 8, 1, 6, 3, 12, 5, 10, 11},
		{0, 9, 14, 11, 4, 13, 2, 3, 8, 1, 6, 15, 12, 5, 10, 7},
		{0, 9, 14, 3, 4, 13, 2, 15, 8, 1, 6, 11, 12, 5, 10, 7},
		{0, 9, 14, 15, 4, 13, 2, 3, 8, 1, 6, 11, 12, 5, 10, 7},
	}[variant]
	initial := []uint32{0xbbb5de72, 0xd4638d5b, 0x92a5f73b, 0x89f8a1c9}[variant]
	initialBytes := hongguoLe32(initial)
	expanded := make([]byte, 48)
	for index := 0; index < 16; index++ {
		expanded[index] = initialBytes[index&3] ^ keyBytes[index]
	}
	rounds := 8
	for index := 4; index < 12; index++ {
		at := 4 * (index - 1)
		a, b, c, d := expanded[at], expanded[at+1], expanded[at+2], expanded[at+3]
		if index&3 == 0 {
			next := byte((initial>>uint(rounds&24))^uint32(box[b])) & 255
			b = box[c]
			c = box[d]
			d = box[a]
			a = next
		}
		rounds += 2
		expanded[at+4] = a ^ expanded[at-12]
		expanded[at+5] = b ^ expanded[at-11]
		expanded[at+6] = c ^ expanded[at-10]
		expanded[at+7] = d ^ expanded[at-9]
	}
	blockEncrypt := func(value []byte) []byte {
		rows := [4][4]byte{}
		for index := 0; index < 4; index++ {
			copy(rows[index][:], value[index*4:index*4+4])
		}
		addCon := func(offset int) {
			for i := 0; i < 4; i++ {
				for j := 0; j < 4; j++ {
					rows[i][j] ^= expanded[(offset+i)*4+con[j]]
				}
			}
		}
		addCon(0)
		for round := 1; round <= 2; round++ {
			for i := 0; i < 4; i++ {
				for j := 0; j < 4; j++ {
					rows[i][j] = box[rows[i][j]]
				}
			}
			before := rows
			for i := 0; i < 4; i++ {
				rows[i] = before[con[i]]
			}
			flat := make([]byte, 16)
			for i := 0; i < 4; i++ {
				copy(flat[i*4:(i+1)*4], rows[i][:])
			}
			for index := 0; index < 16; index++ {
				rows[index/4][index%4] = flat[order[index]]
			}
			if round == 1 {
				snapshot := rows
				for i := 0; i < 4; i++ {
					for j := 0; j < 4; j++ {
						rows[i][j] = snapshot[i][con[j]]
					}
				}
				for index := 0; index < 4; index++ {
					mixed := rows[0][index] ^ rows[1][index] ^ rows[2][index] ^ rows[3][index]
					first := rows[0][index]
					rows[0][index] ^= mixed ^ hongguoXt(rows[0][index]^rows[1][index])
					rows[1][index] ^= mixed ^ hongguoXt(rows[1][index]^rows[2][index])
					rows[2][index] ^= mixed ^ hongguoXt(rows[2][index]^rows[3][index])
					rows[3][index] ^= mixed ^ hongguoXt(rows[3][index]^first)
				}
			}
			addCon(round * 4)
		}
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				rows[i][j] ^= expanded[(4+i)*4+j]
			}
		}
		out := make([]byte, 16)
		for i := 0; i < 4; i++ {
			copy(out[i*4:(i+1)*4], rows[i][:])
		}
		return out
	}
	wire := [8]int{6, 5, 3, 4, 2, 7, 1, 0}
	compact := [8]int{1, 7, 3, 6, 2, 0, 4, 5}
	plaintext := make([]byte, 32)
	for i := 0; i < 31; i++ {
		for j := 0; j < 8; j++ {
			plaintext[i] |= ((sourceBytes[i*8+j] >> uint(wire[j])) & 1) << uint(compact[j])
		}
	}
	plaintext[31] = 1
	input := make([]byte, 16)
	for index := 0; index < 16; index++ {
		input[index] = plaintext[index] ^ ivBytes[index]
	}
	first := blockEncrypt(input)
	for index := 0; index < 16; index++ {
		input[index] = plaintext[index+16] ^ first[index]
	}
	key := append(first, blockEncrypt(input)...)
	out := make([]byte, 0, len(sourceBytes)+1)
	out = append(out, key[31])
	out = append(out, sourceBytes...)
	for i := 0; i < 31; i++ {
		for j := 0; j < 8; j++ {
			at := 1 + i*8 + j
			out[at] = (out[at] &^ (1 << uint(wire[j]))) | (((key[i] >> uint(compact[j])) & 1) << uint(wire[j]))
		}
	}
	return out, nil
}

func hongguoXt(value byte) byte {
	result := (value << 1) & 255
	if value&128 != 0 {
		result ^= 0x1b
	}
	return result
}

func hongguoPlaybackV5Xmxor(data, key []byte) []byte {
	encoded := make([]byte, len(data))
	for index := 0; index < len(data); index++ {
		at := (index * 4) & 28
		d0, d1 := key[at], key[at+1]
		mixed := int32(hongguoRotl8(data[index], 4)) + int32(d0)
		mixed = ^mixed ^ int32(d1)
		mixed = int32(hongguoRotl8(byte(mixed&255), 3))
		mixed = (mixed + int32(d1)) & 255
		mixed ^= int32(d0)
		encoded[len(data)-index-1] = byte(^mixed)
	}
	lastIndex := len(encoded) - 1
	last := encoded[lastIndex] ^ encoded[lastIndex-1]
	first := encoded[0]
	encoded[0] = byte((^(int32(last)) + int32(first)) & 255)
	encoded[1] = byte(((int32(encoded[0] ^ encoded[lastIndex] ^ 254)) + int32(encoded[1])) & 255)
	encoded[2] = byte((int32(encoded[2]) + ((int32(last) - int32(first)) ^ int32(hongguoRotl8(encoded[1], 3)) ^ 2)) & 255)
	for index := 0; index < len(encoded)-4; index++ {
		mixed := int32(hongguoRotl8(encoded[index+2], 3)) ^ int32(encoded[index+1]) ^ int32(index+3)
		encoded[index+3] = byte((^(mixed) + int32(encoded[index+3])) & 255)
	}
	encoded[lastIndex] ^= encoded[lastIndex-1]
	sum := 0
	for index := 1; index < len(encoded); index++ {
		sum += int(encoded[index])
	}
	encoded[0] = byte(((int32(encoded[0] ^ encoded[1])) + int32(sum)) & 255)
	return encoded
}

func hongguoPlaybackV5Helios(stamp uint32) string {
	prefix := make([]byte, 4)
	hongguoRandRead(prefix)
	digest := md5.Sum(append(prefix, []byte("8662")...))
	ascii := []byte(fmt.Sprintf("%x", digest))
	left := hongguoReadU64(ascii, 0)
	right := hongguoReadU64(ascii, 8)
	queue := []uint64{hongguoReadU64(ascii, 16), hongguoReadU64(ascii, 24)}
	var keys []uint64
	for index := 0; index < 34; index++ {
		keys = append(keys, left)
		mixed := hongguoRotl64(right, -8) + left
		mixed ^= uint64(index)
		queue = append(queue, mixed)
		left = mixed ^ hongguoRotl64(left, 3)
		right = queue[0]
		queue = queue[1:]
	}
	data := hongguoPad16([]byte(strconv.FormatUint(uint64(stamp), 10) + "-646637794-8662"))
	for offset := 0; offset < len(data); offset += 16 {
		a := hongguoReadU64(data, offset)
		b := hongguoReadU64(data, offset+8)
		for _, key := range keys {
			b = key ^ (a + hongguoRotl64(b, -8))
			a = b ^ hongguoRotl64(a, 3)
		}
		hongguoWriteU64(data, offset, a)
		hongguoWriteU64(data, offset+8, b)
	}
	return base64.StdEncoding.EncodeToString(append(prefix, data...))
}

func hongguoPlaybackQuery(deviceID, installID string) url.Values {
	return url.Values{
		"aid":                 {"8662"},
		"app_name":            {"novelread"},
		"version_code":        {"74332"},
		"version_name":        {"7.4.3.32"},
		"channel":             {"vivo_8662_64"},
		"device_platform":     {"android"},
		"device_type":         {"PGT-AN10"},
		"os_version":          {"12"},
		"update_version_code": {"74332"},
		"iid":                 {installID},
		"cdid":                {uuidLike()},
		"device_id":           {deviceID},
	}
}

func (downloader *Downloader) hongguoPlaybackProcess(nowMs int64) (string, int64) {
	downloader.hongguoV5Mu.Lock()
	defer downloader.hongguoV5Mu.Unlock()
	if downloader.hongguoV5Process == nil {
		raw := make([]byte, 18)
		hongguoRandRead(raw)
		downloader.hongguoV5Process = &hongguoPlaybackV5Process{
			nonce:   base64.RawURLEncoding.EncodeToString(raw),
			started: nowMs,
		}
	}
	return downloader.hongguoV5Process.nonce, downloader.hongguoV5Process.started
}

func (downloader *Downloader) hongguoPlaybackV5Medusa(queryHash, f13 []byte, stamp uint32, who map[string]string, nowMs int64) string {
	nonce, started := downloader.hongguoPlaybackProcess(nowMs)
	flags := make([]byte, 8)
	deviceInfo := hongguoProtoFields(
		uint64(1), uint64(1), uint64(2), uint64(2), uint64(3), "8662", uint64(4), who["device_id"],
		uint64(5), nonce, uint64(6), "!noperm!", uint64(12), "Asia/Shanghai,8", uint64(13), "zh_CN",
		uint64(22), who["os_version"], uint64(25), uint64(started), uint64(30), who["device_type"],
		uint64(31), who["device_brand"], uint64(32), who["device_type"], uint64(34), who["device_brand"],
	)
	env := hongguoProtoFields(
		uint64(6), "v04.10.03.04-bugfix", uint64(8), who["channel"], uint64(12), deviceInfo, uint64(14), who["version_name"],
	)
	nonceBytes := make([]byte, 4)
	hongguoRandRead(nonceBytes)
	messageNonce := hongguoU32LE(nonceBytes, 0)
	message := hongguoProtoFields(
		uint64(1), hongguoPlaybackV5Magic, uint64(2), uint64(5), uint64(3), uint64(messageNonce), uint64(4), "8662",
		uint64(5), who["device_id"], uint64(6), "646637794", uint64(7), who["version_name"], uint64(8), "v04.10.03-ml-android",
		uint64(9), uint64(0x040a0300), uint64(10), flags, uint64(12), uint64(stamp), uint64(13), f13,
		uint64(14), queryHash[:6], uint64(15), hongguoProtoFields(uint64(1), uint64(1), uint64(2), uint64(1), uint64(3), uint64(1), uint64(5), uint64(1)),
		uint64(17), uint64(stamp), uint64(20), "none", uint64(21), uint64(377), uint64(23), env,
		uint64(24), `{"cmr":16777216,"cmr2":16777216,"un_h":11,"vpn":0,"kd":0,"dyn":"","do":1,"tk":true}`,
	)
	nonceSeed := make([]byte, 4)
	hongguoRandRead(nonceSeed)
	nonce32 := hongguoU32LE(nonceSeed, 0)
	hashSource := append(append([]byte{}, hongguoPlaybackV5SignKey...), hongguoLe32(nonce32)...)
	hash := hongguoSM3(append(hashSource, hongguoPlaybackV5SignKey...))
	high := byte(nonce32 >> 16)
	seedValue := hongguoLe32(^(((uint32(high) << 11) | (nonce32 >> 24)) ^ (uint32(high) >> 5) ^ uint32(high)))
	transformed := append(append([]byte{}, flags...), hongguoPlaybackV5Xmxor(message, hash[:])...)
	for index := 0; index < len(transformed)/2; index++ {
		transformed[index], transformed[len(transformed)-1-index] = transformed[len(transformed)-1-index], transformed[index]
	}
	for index := range transformed {
		transformed[index] ^= seedValue[(^index)&3]
	}
	check := uint32(0x18000001) | (uint32(queryHash[0]&63) << 14) | (uint32(f13[0]&63) << 8)
	packedRandom := make([]byte, 4)
	hongguoRandRead(packedRandom)
	packed := []byte{0xca}
	packed = append(packed, packedRandom...)
	packed = append(packed, hongguoLe32(check)...)
	packed = append(packed, transformed...)
	packed = append(packed, byte(nonce32>>16), byte(nonce32>>24))
	md5Low := md5.Sum(hongguoPlaybackV5SignKey[:16])
	md5High := md5.Sum(hongguoPlaybackV5SignKey[16:])
	encrypted, err := hongguoPlaybackV5Carrier(md5Low[:], md5High[:], packed, stamp)
	if err != nil {
		return ""
	}
	prefix := append(hongguoLe32(5), hongguoPlaybackV5Magic...)
	stampBytes := hongguoLe32(stamp)
	for index := range prefix {
		prefix[index] ^= stampBytes[index&3]
	}
	result := append(prefix, byte(nonce32), byte(nonce32>>8), 0, 1)
	result = append(result, encrypted...)
	return base64.StdEncoding.EncodeToString(result)
}

func (downloader *Downloader) hongguoSignPlaybackV5(base string, values url.Values, body []byte, now time.Time) (string, map[string]string, error) {
	stamp := uint32(now.Unix())
	nowMs := now.UnixMilli()
	bodyMd5 := make([]byte, 16)
	if body != nil {
		sum := md5.Sum(body)
		bodyMd5 = sum[:]
	}
	params := url.Values{}
	for key, values := range values {
		params[key] = append([]string(nil), values...)
	}
	var queryHash []byte
	who := map[string]string{}
	for offset := 0; offset < 1024; offset++ {
		params.Set("_rticket", strconv.FormatInt(nowMs+int64(offset), 10))
		query := params.Encode()
		sum := hongguoSM3([]byte(query))
		queryHash = sum[:]
		if hongguoPlaybackV5IV(queryHash, bodyMd5, stamp)&15 != 12 {
			continue
		}
		for _, name := range []string{"device_id", "version_name", "os_version", "device_type", "channel"} {
			who[name] = params.Get(name)
			if who[name] == "" {
				return "", nil, fmt.Errorf("播放签名缺少设备字段：%s", name)
			}
		}
		who["device_brand"] = firstNonEmpty(params.Get("device_brand"), "HONOR")
		f13, err := hongguoPlaybackV5Hash(queryHash, bodyMd5, stamp)
		if err != nil {
			return "", nil, err
		}
		headers := map[string]string{
			"X-Khronos": strconv.FormatUint(uint64(stamp), 10),
			"X-Helios":  hongguoPlaybackV5Helios(stamp),
			"X-Medusa":  downloader.hongguoPlaybackV5Medusa(queryHash, f13, stamp, who, nowMs),
		}
		if body != nil {
			headers["X-SS-STUB"] = strings.ToUpper(fmt.Sprintf("%x", bodyMd5))
			headers["Content-Type"] = "application/json; charset=utf-8"
		}
		return base + "?" + query, headers, nil
	}
	return "", nil, errors.New("无法在 1024 次 ticket 尝试内生成 V5 播放签名")
}

func hongguoWordAt(words []uint32, index int) uint32 {
	if index >= 0 && index < len(words) {
		return words[index]
	}
	return 0
}
