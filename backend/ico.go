package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"sort"

	"golang.org/x/image/draw"
)

// encodeICO 将任意图像按给定尺寸列表生成多尺寸 ICO。
// 每个尺寸以内嵌 PNG 形式写入（Vista+ 通用方式），尺寸越大越靠后。
// 尺寸为空时默认生成 256x256。
func encodeICO(src image.Image, sizes []int) ([]byte, error) {
	sizes = normalizeSizes(sizes)

	type entry struct {
		data []byte
		w, h byte
	}

	entries := make([]entry, 0, len(sizes))
	for _, s := range sizes {
		img := resizeTo(src, s)

		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}

		// ICO 目录项中的宽高为 1 字节，0 表示 256；>256 时也记为 0，
		// 真实尺寸由内嵌 PNG 的 IHDR 头决定。
		var wh byte
		if s < 256 {
			wh = byte(s)
		}
		entries = append(entries, entry{data: buf.Bytes(), w: wh, h: wh})
	}

	var out bytes.Buffer
	// ICONDIR（6 字节）
	binary.Write(&out, binary.LittleEndian, uint16(0)) // 保留
	binary.Write(&out, binary.LittleEndian, uint16(1)) // 类型：图标
	binary.Write(&out, binary.LittleEndian, uint16(len(entries)))

	// 计算各图像数据偏移：目录头 + N 个目录项
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		out.WriteByte(e.w)
		out.WriteByte(e.h)
		out.WriteByte(0)                                    // 调色板颜色数
		out.WriteByte(0)                                    // 保留
		binary.Write(&out, binary.LittleEndian, uint16(1))  // 色彩平面数
		binary.Write(&out, binary.LittleEndian, uint16(32)) // 每像素位数
		binary.Write(&out, binary.LittleEndian, uint32(len(e.data)))
		binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(e.data)
	}

	for _, e := range entries {
		out.Write(e.data)
	}

	return out.Bytes(), nil
}

// resizeTo 将图像等比缩放到 size×size。
func resizeTo(src image.Image, size int) image.Image {
	b := src.Bounds()
	if b.Dx() == size && b.Dy() == size {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// normalizeSizes 去重、排序并限制在合法范围，空列表回退为默认 256。
func normalizeSizes(sizes []int) []int {
	if len(sizes) == 0 {
		return []int{256}
	}
	seen := make(map[int]bool)
	out := make([]int, 0, len(sizes))
	for _, s := range sizes {
		if s <= 0 || s > 512 || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return []int{256}
	}
	sort.Ints(out)
	return out
}
