package main

import (
	"bytes"
	"fmt"

	govips "github.com/davidbyttow/govips/v2/vips"
	"golang.org/x/image/bmp"
)

// Convert 将输入图片字节转换为目标格式，返回输出字节与对应 MIME 类型。
func Convert(input []byte, target string, quality int, icoSizes []int) ([]byte, string, error) {
	format, ok := lookupFormat(target)
	if !ok {
		return nil, "", fmt.Errorf("不支持的输出格式: %s", target)
	}

	img, err := govips.NewImageFromBuffer(input)
	if err != nil {
		return nil, "", fmt.Errorf("无法解析图片: %w", err)
	}
	defer img.Close()

	// 目标格式不支持透明通道时，将透明背景压平为白色，避免导出报错。
	if !format.SupportsAlpha && img.HasAlpha() {
		if err := img.Flatten(&govips.Color{R: 255, G: 255, B: 255}); err != nil {
			return nil, "", fmt.Errorf("处理透明通道失败: %w", err)
		}
	}

	switch format.ID {
	case "png":
		buf, _, err := img.ExportPng(govips.NewPngExportParams())
		return buf, format.MimeType, err
	case "jpeg":
		p := govips.NewJpegExportParams()
		p.Quality = clampQuality(quality)
		buf, _, err := img.ExportJpeg(p)
		return buf, format.MimeType, err
	case "webp":
		p := govips.NewWebpExportParams()
		p.Quality = clampQuality(quality)
		buf, _, err := img.ExportWebp(p)
		return buf, format.MimeType, err
	case "gif":
		buf, _, err := img.ExportGIF(govips.NewGifExportParams())
		return buf, format.MimeType, err
	case "tiff":
		buf, _, err := img.ExportTiff(govips.NewTiffExportParams())
		return buf, format.MimeType, err
	case "avif":
		p := govips.NewAvifExportParams()
		p.Quality = clampQuality(quality)
		buf, _, err := img.ExportAvif(p)
		return buf, format.MimeType, err
	case "heic":
		p := govips.NewHeifExportParams()
		p.Quality = clampQuality(quality)
		buf, _, err := img.ExportHeif(p)
		return buf, format.MimeType, err
	case "bmp":
		return exportBMP(img)
	case "ico":
		return exportICO(img, icoSizes)
	default:
		return nil, "", fmt.Errorf("未实现的格式: %s", format.ID)
	}
}

// exportBMP 使用纯 Go 的 x/image/bmp 编码器输出 BMP。
func exportBMP(img *govips.ImageRef) ([]byte, string, error) {
	g, err := img.ToGoImage()
	if err != nil {
		return nil, "", fmt.Errorf("读取像素失败: %w", err)
	}
	var buf bytes.Buffer
	if err := bmp.Encode(&buf, g); err != nil {
		return nil, "", fmt.Errorf("BMP 编码失败: %w", err)
	}
	return buf.Bytes(), "image/bmp", nil
}

// exportICO 将图像按指定尺寸列表转为内嵌 PNG 的多尺寸 ICO。
func exportICO(img *govips.ImageRef, sizes []int) ([]byte, string, error) {
	g, err := img.ToGoImage()
	if err != nil {
		return nil, "", fmt.Errorf("读取像素失败: %w", err)
	}
	out, err := encodeICO(g, sizes)
	if err != nil {
		return nil, "", fmt.Errorf("ICO 编码失败: %w", err)
	}
	return out, "image/x-icon", nil
}

// clampQuality 将质量限制在 1~100，<=0 时返回 0（表示使用默认值）。
func clampQuality(q int) int {
	if q <= 0 {
		return 0
	}
	if q > 100 {
		return 100
	}
	return q
}

// ConvertToGIFAnimation 将多张图片按顺序合成一个动画 GIF。
// 各帧精确缩放到第一帧的尺寸（宽高比不同时会轻微变形），
// 垂直堆叠后设置单页高度，使 libvips 将其识别为多帧动画导出。
func ConvertToGIFAnimation(inputs [][]byte, delayMs int) ([]byte, string, error) {
	if len(inputs) < 2 {
		return nil, "", fmt.Errorf("至少需要 2 张图片才能合成动画")
	}

	frames := make([]*govips.ImageRef, 0, len(inputs))
	for _, in := range inputs {
		img, err := govips.NewImageFromBuffer(in)
		if err != nil {
			for _, f := range frames {
				f.Close()
			}
			return nil, "", fmt.Errorf("无法解析图片: %w", err)
		}
		frames = append(frames, img)
	}
	defer func() {
		for _, f := range frames {
			f.Close()
		}
	}()

	// 统一尺寸：以第一帧为基准，精确缩放（不裁剪、无黑边）
	targetW := frames[0].Width()
	targetH := frames[0].Height()
	for i := 1; i < len(frames); i++ {
		f := frames[i]
		if f.Width() != targetW || f.Height() != targetH {
			hScale := float64(targetW) / float64(f.Width())
			vScale := float64(targetH) / float64(f.Height())
			if err := f.ResizeWithVScale(hScale, vScale, govips.KernelLanczos3); err != nil {
				return nil, "", fmt.Errorf("统一尺寸失败: %w", err)
			}
		}
	}

	// 纵向堆叠所有帧（across=1 表示每行一张）
	stack := frames[0]
	if err := stack.ArrayJoin(frames[1:], 1); err != nil {
		return nil, "", fmt.Errorf("拼接帧失败: %w", err)
	}

	// 设置单页高度，使 libvips 将纵向堆叠图按帧切分导出为动画
	if err := stack.SetPageHeight(targetH); err != nil {
		return nil, "", fmt.Errorf("设置帧高度失败: %w", err)
	}

	// 设置每帧延迟（ms），未指定或非法时默认 300ms
	if delayMs <= 0 {
		delayMs = 300
	}
	if delayMs < 20 {
		delayMs = 20
	}
	if delayMs > 5000 {
		delayMs = 5000
	}
	delays := make([]int, len(frames))
	for i := range delays {
		delays[i] = delayMs
	}
	if err := stack.SetPageDelay(delays); err != nil {
		return nil, "", fmt.Errorf("设置帧延迟失败: %w", err)
	}

	buf, _, err := stack.ExportGIF(govips.NewGifExportParams())
	if err != nil {
		return nil, "", fmt.Errorf("GIF 编码失败: %w", err)
	}
	return buf, "image/gif", nil
}
