import { TrimApp } from '@trimjs/web-app'

// 飞牛宿主环境 SDK（桌面 iframe 内嵌时可用）
export const sdk = new TrimApp()

// 支持的图片扩展名（与后端 imageExts 一致）
const IMAGE_EXTS = ['.png', '.jpg', '.jpeg', '.webp', '.gif', '.tiff', '.avif', '.heic', '.bmp', '.ico']

// 是否运行在飞牛宿主环境（桌面 iframe 内嵌）
export function isHostEnv() {
  return !sdk.isStandaloneWeb
}

// 选择图片文件，返回授权文件路径数组
export async function pickImages() {
  if (sdk.isStandaloneWeb) {
    throw new Error('请在飞牛桌面打开应用后使用「从飞牛选择」')
  }
  const result = await sdk.pickUserFile({
    directory: false,
    accept: IMAGE_EXTS,
    sidebarGroup: ['myFiles'],
    title: '选择图片',
    okText: '确认',
  })
  if (!result || result.code !== 0 || !result.data?.length) {
    return []
  }
  return result.data
}

// 选择保存目录，返回授权目录路径
export async function pickDirectory() {
  if (sdk.isStandaloneWeb) {
    throw new Error('请在飞牛桌面打开应用后使用「保存到飞牛」')
  }
  const result = await sdk.pickUserFile({
    directory: true,
    sidebarGroup: ['myFiles'],
    title: '选择保存位置',
    okText: '确认',
  })
  if (!result || result.code !== 0 || !result.data?.length) {
    return ''
  }
  return result.data[0]
}
