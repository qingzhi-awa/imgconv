package main

// Format 描述一种支持的输出图片格式。
type Format struct {
	ID              string `json:"id"`                 // 标识，如 "png"
	Name            string `json:"name"`               // 显示名，如 "PNG"
	Extension       string `json:"extension"`          // 文件扩展名，如 ".png"
	MimeType        string `json:"mimeType"`           // HTTP Content-Type
	SupportsAlpha   bool   `json:"supportsAlpha"`      // 是否支持透明通道（不支持时需先压平透明背景）
	SupportsQuality bool   `json:"supportsQuality"`    // 是否支持质量参数
	IcoSizes        []int  `json:"icoSizes,omitempty"` // 仅 ICO 使用，可选尺寸
}

// outputFormats 为支持的输出格式列表，前端 /api/formats 据此渲染。
var outputFormats = []Format{
	{ID: "png", Name: "PNG", Extension: ".png", MimeType: "image/png", SupportsAlpha: true},
	{ID: "jpeg", Name: "JPEG", Extension: ".jpg", MimeType: "image/jpeg", SupportsQuality: true},
	{ID: "webp", Name: "WebP", Extension: ".webp", MimeType: "image/webp", SupportsAlpha: true, SupportsQuality: true},
	{ID: "gif", Name: "GIF", Extension: ".gif", MimeType: "image/gif", SupportsAlpha: true},
	{ID: "tiff", Name: "TIFF", Extension: ".tiff", MimeType: "image/tiff", SupportsAlpha: true},
	{ID: "avif", Name: "AVIF", Extension: ".avif", MimeType: "image/avif", SupportsAlpha: true, SupportsQuality: true},
	{ID: "heic", Name: "HEIC", Extension: ".heic", MimeType: "image/heic", SupportsAlpha: true, SupportsQuality: true},
	{ID: "bmp", Name: "BMP", Extension: ".bmp", MimeType: "image/bmp"},
	{ID: "ico", Name: "ICO", Extension: ".ico", MimeType: "image/x-icon", SupportsAlpha: true, IcoSizes: []int{16, 32, 48, 64, 128, 192, 256, 512}},
}

// lookupFormat 按 ID 查找输出格式。
func lookupFormat(id string) (Format, bool) {
	for _, f := range outputFormats {
		if f.ID == id {
			return f, true
		}
	}
	return Format{}, false
}
