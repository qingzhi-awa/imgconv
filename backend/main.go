package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	govips "github.com/davidbyttow/govips/v2/vips"
)

const maxUploadBytes = 25 << 20 // 最大上传 25 MB

func main() {
	if err := govips.Startup(nil); err != nil {
		log.Fatalf("libvips 初始化失败: %v", err)
	}
	defer govips.Shutdown()
	log.Printf("libvips %s 已启动", govips.Version)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/formats", handleFormats)
	mux.HandleFunc("POST /api/convert", handleConvert)
	mux.HandleFunc("GET /api/fs/roots", handleFSRoots)
	mux.HandleFunc("POST /api/animate", handleAnimate)
	mux.HandleFunc("POST /api/save", handleSave)
	mux.HandleFunc("GET /api/dirs", handleListDirs)
	mux.HandleFunc("GET /api/browse", handleBrowse)
	mux.HandleFunc("GET /api/read", handleRead)

	// 托管前端静态文件（若已构建），使一个二进制即可同时提供 API 与页面。
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "dist"
	}
	if _, err := os.Stat(staticDir); err == nil {
		mux.Handle("/", http.FileServer(http.Dir(staticDir)))
		log.Printf("已托管前端静态文件: %s", staticDir)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "前端未构建：请在 frontend 目录运行 npm run build", http.StatusNotFound)
		})
	}

	handler := withCORS(mux)

	// TCP 监听：直接访问 / 局域网访问用
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	go func() {
		log.Printf("监听 TCP :%s", port)
		if err := http.ListenAndServe(":"+port, handler); err != nil {
			log.Fatalf("TCP 服务退出: %v", err)
		}
	}()

	// Unix Socket 监听：飞牛统一网关（网关只转发到 target 目录下的 app.sock）
	if sock := os.Getenv("GATEWAY_SOCK"); sock != "" {
		_ = os.Remove(sock)
		sockLn, err := net.Listen("unix", sock)
		if err != nil {
			log.Fatalf("Unix Socket 监听失败: %v", err)
		}
		_ = os.Chmod(sock, 0o666)
		log.Printf("统一网关 Socket 已监听: %s", sock)
		go func() {
			// 网关转发时保留 /app/imgconv 前缀，这里剥离后再交给路由
			if err := http.Serve(sockLn, http.StripPrefix("/app/imgconv", handler)); err != nil {
				log.Fatalf("Socket 服务退出: %v", err)
			}
		}()
	}

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，正在关闭...")
}

// handleHealth 返回服务健康状态。
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": govips.Version,
	})
}

// handleFormats 返回支持的输出格式列表。
func handleFormats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"formats": outputFormats})
}

// handleConvert 接收上传图片并转换为目标格式。
func handleConvert(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		httpError(w, http.StatusBadRequest, "文件过大或表单解析失败")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, http.StatusBadRequest, "缺少上传文件")
		return
	}
	defer file.Close()

	input, err := io.ReadAll(file)
	if err != nil {
		httpError(w, http.StatusBadRequest, "读取文件失败")
		return
	}
	if len(input) == 0 {
		httpError(w, http.StatusBadRequest, "文件为空")
		return
	}

	target := strings.ToLower(strings.TrimSpace(r.FormValue("format")))
	quality, _ := strconv.Atoi(r.FormValue("quality"))

	var icoSizes []int
	if target == "ico" {
		icoSizes = parseSizes(r.FormValue("sizes"))
	}

	output, mime, err := Convert(input, target, quality, icoSizes)
	if err != nil {
		httpError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	format, _ := lookupFormat(target)
	filename := baseNameWithoutExt(header.Filename) + format.Extension

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(filename)))
	w.Header().Set("Content-Length", strconv.Itoa(len(output)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output)
}

// handleAnimate 接收多张图片，按上传顺序合成一个动画 GIF。
func handleAnimate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		httpError(w, http.StatusBadRequest, "文件过大或表单解析失败")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) < 2 {
		httpError(w, http.StatusBadRequest, "请至少选择 2 张图片")
		return
	}

	inputs := make([][]byte, 0, len(files))
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			httpError(w, http.StatusBadRequest, "读取文件失败")
			return
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			httpError(w, http.StatusBadRequest, "读取文件失败")
			return
		}
		if len(data) == 0 {
			httpError(w, http.StatusBadRequest, "文件为空")
			return
		}
		inputs = append(inputs, data)
	}

	delay, _ := strconv.Atoi(r.FormValue("delay"))
	output, mime, err := ConvertToGIFAnimation(inputs, delay)
	if err != nil {
		httpError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", `attachment; filename="animation.gif"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(output)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output)
}

// handleSave 将转换结果保存到飞牛磁盘（而非下载到浏览器）。
func handleSave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		httpError(w, http.StatusBadRequest, "文件过大或表单解析失败")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, http.StatusBadRequest, "缺少文件")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		httpError(w, http.StatusBadRequest, "读取文件失败")
		return
	}
	if len(data) == 0 {
		httpError(w, http.StatusBadRequest, "文件为空")
		return
	}

	dir := r.FormValue("dir")
	if dir == "" {
		dir = saveDir()
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpError(w, http.StatusInternalServerError, "创建保存目录失败")
		return
	}

	filename := sanitizeFilename(header.Filename)
	if filename == "" {
		filename = "converted"
	}
	dst := filepath.Join(dir, filename)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		httpError(w, http.StatusInternalServerError, "保存文件失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": dst})
}

// saveDir 返回"保存到飞牛"的目标目录，可通过 SAVE_DIR 覆盖。
func saveDir() string {
	if d := os.Getenv("SAVE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("TRIM_PKGVAR"); d != "" {
		return filepath.Join(d, "saved")
	}
	return "saved"
}

// handleListDirs 列出指定目录下的子目录，供前端目录选择器使用。
func handleListDirs(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = saveRoot()
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		httpError(w, http.StatusBadRequest, "路径必须是绝对路径")
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		httpError(w, http.StatusBadRequest, "无法读取目录："+err.Error())
		return
	}

	type dirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	subdirs := make([]dirEntry, 0)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			subdirs = append(subdirs, dirEntry{Name: e.Name(), Path: filepath.Join(dir, e.Name())})
		}
	}
	sort.Slice(subdirs, func(i, j int) bool { return subdirs[i].Name < subdirs[j].Name })

	writeJSON(w, http.StatusOK, map[string]any{
		"current": dir,
		"parent":  filepath.Dir(dir),
		"dirs":    subdirs,
	})
}

// saveRoot 返回目录浏览的默认根目录，可通过 SAVE_ROOT 覆盖。
func saveRoot() string {
	if d := os.Getenv("SAVE_ROOT"); d != "" {
		return d
	}
	if _, err := os.Stat("/vol1"); err == nil {
		return "/vol1"
	}
	return "/"
}

// imageExts 支持的图片扩展名，用于"从飞牛选图"时筛选文件。
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true,
	".tiff": true, ".avif": true, ".heic": true, ".heif": true, ".bmp": true,
	".ico": true, ".svg": true,
}

func isImageFile(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

// handleBrowse 列出指定目录下的子目录与图片文件，供前端文件选择器使用。
func handleBrowse(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = saveRoot()
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		httpError(w, http.StatusBadRequest, "路径必须是绝对路径")
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		httpError(w, http.StatusBadRequest, "无法读取目录："+err.Error())
		return
	}

	type entry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}
	dirs := make([]entry, 0)
	files := make([]entry, 0)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			dirs = append(dirs, entry{Name: e.Name(), Path: p, IsDir: true})
		} else if isImageFile(e.Name()) {
			files = append(files, entry{Name: e.Name(), Path: p, IsDir: false})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	writeJSON(w, http.StatusOK, map[string]any{
		"current": dir,
		"parent":  filepath.Dir(dir),
		"dirs":    dirs,
		"files":   files,
	})
}

// handleRead 读取飞牛上指定文件的内容（供前端把飞牛图片拉入上传列表）。
func handleRead(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		httpError(w, http.StatusBadRequest, "缺少文件路径")
		return
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) {
		httpError(w, http.StatusBadRequest, "路径必须是绝对路径")
		return
	}
	if !isImageFile(p) {
		httpError(w, http.StatusBadRequest, "不是支持的图片文件")
		return
	}

	data, err := os.ReadFile(p)
	if err != nil {
		httpError(w, http.StatusNotFound, "文件不存在或无法读取")
		return
	}

	ct := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png":
		ct = "image/png"
	case ".jpg", ".jpeg":
		ct = "image/jpeg"
	case ".webp":
		ct = "image/webp"
	case ".gif":
		ct = "image/gif"
	case ".tiff":
		ct = "image/tiff"
	case ".avif":
		ct = "image/avif"
	case ".heic", ".heif":
		ct = "image/heic"
	case ".bmp":
		ct = "image/bmp"
	case ".ico":
		ct = "image/x-icon"
	case ".svg":
		ct = "image/svg+xml"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// parseSizes 解析逗号分隔的 ICO 尺寸列表，去重并升序。
func parseSizes(s string) []int {
	if s == "" {
		return nil
	}
	seen := make(map[int]bool)
	var out []int
	for _, p := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 || n > 512 || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// baseNameWithoutExt 去掉路径与扩展名，返回纯文件名。
func baseNameWithoutExt(name string) string {
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	if base == "" {
		base = "converted"
	}
	return base
}

// sanitizeFilename 移除可能导致 Content-Disposition 注入的字符。
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	return name
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// withCORS 为本地开发跨域（Vite dev server）提供基础 CORS 支持。
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
