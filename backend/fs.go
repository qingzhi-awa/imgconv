package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FSNode 表示文件选择器中的一个入口。
type FSNode struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// handleFSRoots 返回文件选择器的分类入口。
func handleFSRoots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"roots": discoverRoots(currentUID(r))})
}

// currentUID 从飞牛统一网关注入的 X-trim-users 头提取当前登录用户的 uid。
// 该头仅在经网关（app.sock）访问时存在；TCP 直连时为空，返回 ""。
func currentUID(r *http.Request) string {
	uid := strings.TrimSpace(r.Header.Get("X-trim-users"))
	if uid != "" && isNumeric(uid) {
		return uid
	}
	return ""
}

// userRoot 返回当前用户的家目录（/vol*/<uid>），找不到返回 ""。
func userRoot(uid string) string {
	if uid == "" {
		return ""
	}
	rootEntries, _ := os.ReadDir("/")
	for _, e := range rootEntries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "vol") {
			p := "/" + e.Name() + "/" + uid
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// checkAccess 校验路径是否在当前用户目录下；uid 为空（TCP 直连）时不鉴权。
func checkAccess(r *http.Request, path string) bool {
	uid := currentUID(r)
	if uid == "" {
		return true
	}
	root := userRoot(uid)
	if root == "" {
		return false
	}
	path = filepath.Clean(path)
	return path == root || strings.HasPrefix(path, root+"/")
}

// defaultRoot 返回文件浏览的默认根目录；已鉴权时优先返回当前用户目录。
func defaultRoot(r *http.Request) string {
	if uid := currentUID(r); uid != "" {
		if root := userRoot(uid); root != "" {
			return root
		}
	}
	return saveRoot()
}

// isDataVol 判断是否为数据存储卷（vol1、vol2…，排除 vol0 等系统卷）。
func isDataVol(name string) bool {
	if !strings.HasPrefix(name, "vol") {
		return false
	}
	num := strings.TrimPrefix(name, "vol")
	return num != "" && isNumeric(num) && num != "0"
}

// discoverRoots 扫描飞牛存储卷，构造文件选择器侧栏入口。
// uid 非空时只返回该用户自己的目录；为空时返回存储空间与全部用户目录。
func discoverRoots(uid string) []FSNode {
	var roots []FSNode

	var vols []string
	rootEntries, _ := os.ReadDir("/")
	for _, e := range rootEntries {
		if e.IsDir() && isDataVol(e.Name()) {
			vols = append(vols, "/"+e.Name())
		}
	}
	sort.Strings(vols)

	// 鉴权模式：只返回当前用户目录
	if uid != "" {
		for _, v := range vols {
			p := v + "/" + uid
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				roots = append(roots, FSNode{Name: "我的文件", Path: p, IsDir: true})
			}
		}
		return roots
	}

	// 非鉴权：存储空间根 + 全部用户目录
	for _, v := range vols {
		roots = append(roots, FSNode{Name: "存储空间 " + strings.TrimPrefix(v, "/vol"), Path: v, IsDir: true})
	}
	for _, v := range vols {
		entries, _ := os.ReadDir(v)
		var uids []string
		for _, e := range entries {
			if e.IsDir() && isNumeric(e.Name()) {
				uids = append(uids, e.Name())
			}
		}
		sort.Strings(uids)
		for _, u := range uids {
			roots = append(roots, FSNode{Name: "我的文件 " + u, Path: v + "/" + u, IsDir: true})
		}
	}
	return roots
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
