<script setup>
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { fetchFormats, convertImage, animateImages, downloadBlob, saveToServer, listDirs, browseFnos, readFnosFile, fetchFSRoots } from './api'

const formats = ref([])
const files = ref([])
const previews = ref([])
const selectedId = ref('')
const quality = ref(80)
const icoSizes = ref([256])
const delay = ref(300)
const converting = ref(false)
const error = ref('')
const done = ref('')
const fileInput = ref(null)
const resultUrl = ref('')
const resultBlob = ref(null)
const resultFilename = ref('')
const saving = ref(false)
const pickerOpen = ref(false)
const pickerCurrent = ref('')
const pickerParent = ref('')
const pickerDirs = ref([])
const pickerLoading = ref(false)
const filePickerOpen = ref(false)
const filePickerCurrent = ref('')
const filePickerParent = ref('')
const filePickerDirs = ref([])
const filePickerFiles = ref([])
const filePickerLoading = ref(false)
const roots = ref([])

const selectedFormat = computed(() =>
  formats.value.find((f) => f.id === selectedId.value) || null
)

// GIF 支持多张图片合成动画
const isAnimated = computed(() => selectedFormat.value?.id === 'gif')
// 真正走动画合成：GIF 且选中了 2 张以上
const multi = computed(() => isAnimated.value && files.value.length >= 2)

fetchFormats()
  .then((list) => {
    formats.value = list
    if (list.length) selectedId.value = list[0].id
  })
  .catch((e) => (error.value = e.message))

// 切换到非 GIF 格式时，只保留第一张
watch(isAnimated, (animated) => {
  if (!animated && files.value.length > 1) {
    previews.value.slice(1).forEach((u) => URL.revokeObjectURL(u))
    files.value = files.value.slice(0, 1)
    previews.value = previews.value.slice(0, 1)
  }
})

function pick() {
  fileInput.value?.click()
}

function revokeAll() {
  previews.value.forEach((u) => URL.revokeObjectURL(u))
  previews.value = []
}

function addFiles(list) {
  const arr = Array.from(list || [])
  if (!arr.length) return
  error.value = ''
  done.value = ''
  if (isAnimated.value) {
    for (const f of arr) {
      files.value.push(f)
      previews.value.push(URL.createObjectURL(f))
    }
  } else {
    revokeAll()
    files.value = [arr[0]]
    previews.value = [URL.createObjectURL(arr[0])]
  }
}

function onDrop(e) {
  addFiles(e.dataTransfer?.files)
}

function onPick(e) {
  addFiles(e.target.files)
  e.target.value = ''
}

function toggleSize(size) {
  const i = icoSizes.value.indexOf(size)
  if (i >= 0) {
    icoSizes.value.splice(i, 1)
  } else {
    icoSizes.value.push(size)
  }
  icoSizes.value.sort((a, b) => a - b)
}

function baseName(name) {
  const base = name.replace(/\\/g, '/').split('/').pop() || 'converted'
  const i = base.lastIndexOf('.')
  return i > 0 ? base.slice(0, i) : base
}

function removeAt(i) {
  URL.revokeObjectURL(previews.value[i])
  files.value.splice(i, 1)
  previews.value.splice(i, 1)
}

async function convert() {
  if (!files.value.length || !selectedFormat.value) return
  converting.value = true
  error.value = ''
  done.value = ''
  try {
    let blob
    let filename
    if (multi.value) {
      blob = await animateImages(files.value, delay.value)
      filename = 'animation.gif'
    } else {
      blob = await convertImage(
        files.value[0],
        selectedFormat.value.id,
        selectedFormat.value.supportsQuality ? quality.value : null,
        icoSizes.value
      )
      filename = `${baseName(files.value[0].name)}${selectedFormat.value.extension}`
    }
    resultBlob.value = blob
    resultFilename.value = filename
    if (resultUrl.value) URL.revokeObjectURL(resultUrl.value)
    resultUrl.value = URL.createObjectURL(blob)
    done.value = `转换完成：${filename}`
  } catch (e) {
    error.value = e.message
  } finally {
    converting.value = false
  }
}

function reset() {
  revokeAll()
  files.value = []
  error.value = ''
  done.value = ''
  if (resultUrl.value) URL.revokeObjectURL(resultUrl.value)
  resultUrl.value = ''
  resultBlob.value = null
  resultFilename.value = ''
}

function downloadLocal() {
  if (resultBlob.value) downloadBlob(resultBlob.value, resultFilename.value)
}

async function loadRoots() {
  if (roots.value.length) return
  try {
    roots.value = await fetchFSRoots()
  } catch (_) {
    /* 侧栏加载失败不影响主流程 */
  }
}

function pathParts(p) {
  if (!p || p === '/') return []
  return p.split('/').filter(Boolean).map((seg, i, arr) => ({
    name: seg,
    path: '/' + arr.slice(0, i + 1).join('/')
  }))
}

function isActiveRoot(path, rootPath) {
  if (!rootPath || rootPath === '/') return path === rootPath
  return path === rootPath || path.startsWith(rootPath + '/')
}

async function saveToNas() {
  if (!resultBlob.value) return
  pickerOpen.value = true
  loadRoots()
  await loadDirs('')
}

async function loadDirs(path) {
  pickerLoading.value = true
  try {
    const data = await listDirs(path)
    pickerCurrent.value = data.current
    pickerParent.value = data.parent
    pickerDirs.value = data.dirs
  } catch (e) {
    error.value = e.message
  } finally {
    pickerLoading.value = false
  }
}

function enterDir(path) {
  loadDirs(path)
}

function goParent() {
  if (pickerParent.value) loadDirs(pickerParent.value)
}

function closePicker() {
  pickerOpen.value = false
}

async function confirmSave() {
  const dir = pickerCurrent.value
  pickerOpen.value = false
  saving.value = true
  error.value = ''
  done.value = ''
  try {
    const p = await saveToServer(resultBlob.value, resultFilename.value, dir)
    done.value = `已保存到飞牛：${p}`
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

async function openFilePicker() {
  filePickerOpen.value = true
  loadRoots()
  await loadFilePicker('')
}

async function loadFilePicker(path) {
  filePickerLoading.value = true
  try {
    const data = await browseFnos(path)
    filePickerCurrent.value = data.current
    filePickerParent.value = data.parent
    filePickerDirs.value = data.dirs
    filePickerFiles.value = data.files
  } catch (e) {
    error.value = e.message
  } finally {
    filePickerLoading.value = false
  }
}

function enterFileDir(path) {
  loadFilePicker(path)
}

function goFileParent() {
  if (filePickerParent.value) loadFilePicker(filePickerParent.value)
}

function closeFilePicker() {
  filePickerOpen.value = false
}

async function selectFnosFile(path, name) {
  closeFilePicker()
  try {
    const blob = await readFnosFile(path)
    const file = new File([blob], name, { type: blob.type || 'image/*' })
    addFiles([file])
    done.value = ''
  } catch (e) {
    error.value = e.message
  }
}

onBeforeUnmount(() => {
  revokeAll()
  if (resultUrl.value) URL.revokeObjectURL(resultUrl.value)
})
</script>

<template>
  <div class="page">
    <header class="header">
      <h1>图片格式转换</h1>
      <p>支持 PNG / JPEG / WebP / GIF / TIFF / AVIF / HEIC / BMP / ICO</p>
    </header>

    <main class="card">
      <!-- 上传区 -->
      <section
        class="dropzone"
        :class="{ 'has-file': files.length }"
        @dragover.prevent
        @drop.prevent="onDrop"
        @click="!files.length && pick()"
      >
        <input
          ref="fileInput"
          type="file"
          accept="image/*"
          :multiple="isAnimated"
          hidden
          @change="onPick"
        />

        <template v-if="!files.length">
          <div class="icon">＋</div>
          <p class="hint">
            {{ isAnimated ? '拖拽多张图片到此处，按顺序合成动画 GIF' : '拖拽图片到此处，或点击选择文件' }}
          </p>
          <div class="drop-actions">
            <button type="button" class="ghost" @click.stop="pick">本地上传</button>
            <button type="button" class="ghost" @click.stop="openFilePicker">从飞牛选择</button>
          </div>
        </template>

        <template v-else>
          <div class="previews">
            <figure v-for="(url, i) in previews" :key="i" class="preview-item">
              <img :src="url" alt="预览" />
              <figcaption class="file-meta">
                <span class="name">{{ files[i].name }}</span>
                <span class="size">{{ (files[i].size / 1024).toFixed(1) }} KB</span>
                <button type="button" class="ghost" @click.stop="removeAt(i)">移除</button>
              </figcaption>
            </figure>
          </div>
          <div class="drop-actions">
            <button type="button" class="ghost" @click.stop="pick">
              {{ isAnimated ? '继续添加' : '重新选择' }}
            </button>
            <button type="button" class="ghost" @click.stop="reset">清空</button>
          </div>
        </template>
      </section>

      <!-- 格式选择 -->
      <section class="panel">
        <label class="label">目标格式</label>
        <div class="grid">
          <button
            v-for="f in formats"
            :key="f.id"
            type="button"
            class="fmt"
            :class="{ active: f.id === selectedId }"
            @click="selectedId = f.id"
          >
            <span class="fmt-name">{{ f.name }}</span>
            <span class="fmt-ext">{{ f.extension }}</span>
          </button>
        </div>
        <p v-if="isAnimated" class="tip">GIF 支持多张图片合成为动画，可点击「继续添加」按顺序追加图片。</p>
      </section>

      <!-- ICO 尺寸（仅 ICO 格式显示） -->
      <section v-if="selectedFormat?.icoSizes" class="panel">
        <label class="label">图标尺寸（可多选，未选则 256×256）</label>
        <div class="sizes">
          <button
            v-for="s in selectedFormat.icoSizes"
            :key="s"
            type="button"
            class="size-chip"
            :class="{ active: icoSizes.includes(s) }"
            @click="toggleSize(s)"
          >
            {{ s }}×{{ s }}
          </button>
        </div>
      </section>

      <!-- 质量 -->
      <section v-if="selectedFormat?.supportsQuality" class="panel">
        <div class="quality-head">
          <label class="label">质量</label>
          <span class="quality-val">{{ quality }}</span>
        </div>
        <input v-model.number="quality" type="range" min="1" max="100" />
      </section>

      <!-- 每帧间隔（仅 GIF 多帧合成时显示） -->
      <section v-if="multi" class="panel">
        <div class="quality-head">
          <label class="label">每帧间隔</label>
          <span class="quality-val">{{ delay }} ms</span>
        </div>
        <input v-model.number="delay" type="range" min="20" max="2000" step="10" />
      </section>

      <!-- 操作 -->
      <section class="actions">
        <button
          class="primary"
          :disabled="!files.length || converting"
          @click="convert"
        >
          {{ converting ? '转换中…' : multi ? '合成动画 GIF' : '开始转换' }}
        </button>
      </section>

      <!-- 转换结果预览 -->
      <section v-if="resultUrl" class="panel result-panel">
        <label class="label">转换结果</label>
        <img :src="resultUrl" class="result-img" alt="转换结果" />
        <div class="result-actions">
          <button type="button" class="ghost" @click="downloadLocal">下载到本地</button>
          <button type="button" class="primary" :disabled="saving" @click="saveToNas">
            {{ saving ? '保存中…' : '保存到飞牛' }}
          </button>
        </div>
      </section>

      <p v-if="error" class="msg error">{{ error }}</p>
      <p v-else-if="done" class="msg done">{{ done }}</p>
    </main>

    <!-- 目录选择弹窗 -->
    <div v-if="pickerOpen" class="modal-mask" @click.self="closePicker">
      <div class="modal picker-modal">
        <div class="modal-head">
          <span class="modal-title">选择保存位置</span>
          <button type="button" class="modal-close" @click="closePicker">×</button>
        </div>
        <div class="modal-body">
          <aside class="picker-side">
            <button
              v-for="r in roots"
              :key="r.path"
              type="button"
              class="side-item"
              :class="{ active: isActiveRoot(pickerCurrent, r.path) }"
              @click="enterDir(r.path)"
            >
              <svg class="side-icon" viewBox="0 0 24 24" fill="none">
                <rect x="3" y="6" width="18" height="12" rx="2" stroke="currentColor" stroke-width="1.6" />
                <circle cx="7" cy="12" r="1.2" fill="currentColor" />
                <path d="M14 12h7" stroke="currentColor" stroke-width="1.6" />
              </svg>
              <span>{{ r.name }}</span>
            </button>
          </aside>
          <div class="picker-main">
            <div class="crumbs">
              <button type="button" class="crumb up" :disabled="!pickerParent" @click="goParent">↑</button>
              <template v-for="(p, i) in pathParts(pickerCurrent)" :key="p.path">
                <span v-if="i > 0" class="crumb-sep">/</span>
                <button type="button" class="crumb" @click="enterDir(p.path)">{{ p.name }}</button>
              </template>
            </div>
            <div class="modal-list">
              <p v-if="pickerLoading" class="msg">加载中…</p>
              <p v-else-if="!pickerDirs.length" class="msg empty">此目录下没有子文件夹</p>
              <button v-for="d in pickerDirs" :key="d.path" type="button" class="dir-item" @click="enterDir(d.path)">
                <svg class="file-icon folder" viewBox="0 0 24 24">
                  <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" fill="#f6b73c" />
                  <path d="M3 10h18" stroke="#fff" stroke-width="1.4" />
                </svg>
                <span class="item-name">{{ d.name }}</span>
              </button>
            </div>
          </div>
        </div>
        <div class="modal-foot">
          <button type="button" class="ghost" @click="closePicker">取消</button>
          <button type="button" class="primary" @click="confirmSave">保存到此处</button>
        </div>
      </div>
    </div>

    <!-- 从飞牛选择图片弹窗 -->
    <div v-if="filePickerOpen" class="modal-mask" @click.self="closeFilePicker">
      <div class="modal picker-modal">
        <div class="modal-head">
          <span class="modal-title">从飞牛选择图片</span>
          <button type="button" class="modal-close" @click="closeFilePicker">×</button>
        </div>
        <div class="modal-body">
          <aside class="picker-side">
            <button
              v-for="r in roots"
              :key="r.path"
              type="button"
              class="side-item"
              :class="{ active: isActiveRoot(filePickerCurrent, r.path) }"
              @click="enterFileDir(r.path)"
            >
              <svg class="side-icon" viewBox="0 0 24 24" fill="none">
                <rect x="3" y="6" width="18" height="12" rx="2" stroke="currentColor" stroke-width="1.6" />
                <circle cx="7" cy="12" r="1.2" fill="currentColor" />
                <path d="M14 12h7" stroke="currentColor" stroke-width="1.6" />
              </svg>
              <span>{{ r.name }}</span>
            </button>
          </aside>
          <div class="picker-main">
            <div class="crumbs">
              <button type="button" class="crumb up" :disabled="!filePickerParent" @click="goFileParent">↑</button>
              <template v-for="(p, i) in pathParts(filePickerCurrent)" :key="p.path">
                <span v-if="i > 0" class="crumb-sep">/</span>
                <button type="button" class="crumb" @click="enterFileDir(p.path)">{{ p.name }}</button>
              </template>
            </div>
            <div class="modal-list">
              <p v-if="filePickerLoading" class="msg">加载中…</p>
              <template v-else>
                <p v-if="!filePickerDirs.length && !filePickerFiles.length" class="msg empty">此目录下没有文件夹或图片</p>
                <button v-for="d in filePickerDirs" :key="d.path" type="button" class="dir-item" @click="enterFileDir(d.path)">
                  <svg class="file-icon folder" viewBox="0 0 24 24">
                    <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" fill="#f6b73c" />
                    <path d="M3 10h18" stroke="#fff" stroke-width="1.4" />
                  </svg>
                  <span class="item-name">{{ d.name }}</span>
                </button>
                <button v-for="f in filePickerFiles" :key="f.path" type="button" class="dir-item" @click="selectFnosFile(f.path, f.name)">
                  <svg class="file-icon image" viewBox="0 0 24 24">
                    <rect x="3" y="4" width="18" height="16" rx="2" fill="#7c8db5" />
                    <circle cx="8.5" cy="9" r="1.6" fill="#fff" />
                    <path d="M4 18l5-5 3 3 3-3 5 5" stroke="#fff" stroke-width="1.6" fill="none" stroke-linecap="round" stroke-linejoin="round" />
                  </svg>
                  <span class="item-name">{{ f.name }}</span>
                </button>
              </template>
            </div>
          </div>
        </div>
        <div class="modal-foot">
          <button type="button" class="ghost" @click="closeFilePicker">取消</button>
        </div>
      </div>
    </div>
  </div>
</template>
