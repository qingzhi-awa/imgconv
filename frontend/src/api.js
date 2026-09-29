// 后端 API 封装。使用相对路径，兼容飞牛网关前缀（/app/imgconv）与直连/本地开发。
const BASE = 'api'

export async function fetchFormats() {
  const res = await fetch(`${BASE}/formats`)
  if (!res.ok) throw new Error('获取格式列表失败')
  const data = await res.json()
  return data.formats
}

export async function convertImage(file, format, quality, icoSizes) {
  const form = new FormData()
  form.append('file', file)
  form.append('format', format)
  if (quality != null) form.append('quality', String(quality))
  if (icoSizes && icoSizes.length) form.append('sizes', icoSizes.join(','))

  const res = await fetch(`${BASE}/convert`, { method: 'POST', body: form })
  if (!res.ok) {
    let msg = '转换失败'
    try {
      const data = await res.json()
      if (data && data.error) msg = data.error
    } catch (_) {
      /* 忽略解析错误 */
    }
    throw new Error(msg)
  }
  return res.blob()
}

export async function animateImages(files, delay) {
  const form = new FormData()
  for (const f of files) form.append('files', f)
  if (delay != null) form.append('delay', String(delay))

  const res = await fetch(`${BASE}/animate`, { method: 'POST', body: form })
  if (!res.ok) {
    let msg = '合成失败'
    try {
      const data = await res.json()
      if (data && data.error) msg = data.error
    } catch (_) {
      /* 忽略解析错误 */
    }
    throw new Error(msg)
  }
  return res.blob()
}

export async function fetchFSRoots() {
  const res = await fetch(`${BASE}/fs/roots`)
  if (!res.ok) throw new Error('获取存储入口失败')
  const data = await res.json()
  return data.roots
}

export async function listDirs(path) {
  const res = await fetch(`${BASE}/dirs?path=${encodeURIComponent(path || '')}`)
  if (!res.ok) throw new Error('读取目录失败')
  return res.json()
}

export async function browseFnos(path) {
  const res = await fetch(`${BASE}/browse?path=${encodeURIComponent(path || '')}`)
  if (!res.ok) throw new Error('读取目录失败')
  return res.json()
}

export async function readFnosFile(path) {
  const res = await fetch(`${BASE}/read?path=${encodeURIComponent(path)}`)
  if (!res.ok) throw new Error('读取文件失败')
  return res.blob()
}

export async function saveToServer(blob, filename, dir) {
  const form = new FormData()
  form.append('file', blob, filename)
  if (dir) form.append('dir', dir)

  const res = await fetch(`${BASE}/save`, { method: 'POST', body: form })
  if (!res.ok) {
    let msg = '保存失败'
    try {
      const data = await res.json()
      if (data && data.error) msg = data.error
    } catch (_) {
      /* 忽略解析错误 */
    }
    throw new Error(msg)
  }
  const data = await res.json()
  return data.path
}

export function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
