package main

import (
	"net/http"
	"os"
	"sort"
	"strings"
)

// FSNode 表示文件选择器分类侧栏中的一个入口。
type FSNode struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// handleFSRoots 返回文件选择器的分类入口（存储空间 / 我的文件 / 团队文件）。
func handleFSRoots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"roots": discoverRoots()})
}

// discoverRoots 扫描飞牛存储卷，构造文件选择器侧栏入口。
func discoverRoots() []FSNode {
	var roots []FSNode

	rootEntries, err := os.ReadDir("/")
	if err != nil {
		return roots
	}
	var vols []string
	for _, e := range rootEntries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "vol") {
			vols = append(vols, "/"+e.Name())
		}
	}
	sort.Strings(vols)

	for _, v := range vols {
		roots = append(roots, FSNode{
			Name:  "存储空间 " + strings.TrimPrefix(v, "/vol"),
			Path:  v,
			IsDir: true,
		})
	}

	if len(vols) > 0 {
		base := vols[0]
		entries, _ := os.ReadDir(base)
		var uids []string
		for _, e := range entries {
			if e.IsDir() && isNumeric(e.Name()) {
				uids = append(uids, e.Name())
			}
		}
		sort.Strings(uids)
		for _, u := range uids {
			roots = append(roots, FSNode{Name: "我的文件 " + u, Path: base + "/" + u, IsDir: true})
		}
		if _, err := os.Stat(base + "/@team"); err == nil {
			roots = append(roots, FSNode{Name: "团队文件", Path: base + "/@team", IsDir: true})
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
