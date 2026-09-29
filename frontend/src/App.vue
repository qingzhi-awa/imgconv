<script setup>
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { fetchFormats, convertImage, animateImages, downloadBlob, saveToServer, readFnosFile } from './api'
import { pickImages, pickDirectory } from './sdk'

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

// 从飞牛选择图片（调用飞牛官方文件选择器，选择后自动授权）
async function openFnosPicker() {
  error.value = ''
  done.value = ''
  try {
    const paths = await pickImages()
    if (!paths.length) return
    const picked = []
    for (const p of paths) {
      const blob = await readFnosFile(p)
      const name = p.split('/').pop() || 'image.png'
      picked.push(new File([blob], name, { type: blob.type || 'image/*' }))
    }
    addFiles(picked)
  } catch (e) {
    error.value = e.message
  }
}

// 保存到飞牛（选择目录后自动授权，再写入）
async function saveToNas() {
  if (!resultBlob.value) return
  saving.value = true
  error.value = ''
  done.value = ''
  try {
    const dir = await pickDirectory()
    if (!dir) return
    const p = await saveToServer(resultBlob.value, resultFilename.value, dir)
    done.value = `已保存到飞牛：${p}`
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
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
            <button type="button" class="ghost" @click.stop="openFnosPicker">从飞牛选择</button>
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
  </div>
</template>
