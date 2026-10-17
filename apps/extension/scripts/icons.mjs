// Generates the toolbar and store icons as PNG files without external tools: a teal rounded
// square with a white capture frame, rendered with 4x4 supersampling.
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { deflateSync } from 'node:zlib'

const CRC_TABLE = new Uint32Array(256).map((_, n) => {
  let c = n
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
  return c >>> 0
})

function crc32(buf) {
  let c = 0xffffffff
  for (const b of buf) c = CRC_TABLE[(c ^ b) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

function chunk(type, data) {
  const len = Buffer.alloc(4)
  len.writeUInt32BE(data.length)
  const td = Buffer.concat([Buffer.from(type, 'ascii'), data])
  const crc = Buffer.alloc(4)
  crc.writeUInt32BE(crc32(td))
  return Buffer.concat([len, td, crc])
}

function encodePng(size, rgba) {
  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(size, 0)
  ihdr.writeUInt32BE(size, 4)
  ihdr[8] = 8 // bit depth
  ihdr[9] = 6 // RGBA
  const raw = Buffer.alloc((size * 4 + 1) * size)
  for (let y = 0; y < size; y++) {
    raw[y * (size * 4 + 1)] = 0
    rgba.copy(raw, y * (size * 4 + 1) + 1, y * size * 4, (y + 1) * size * 4)
  }
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', deflateSync(raw, { level: 9 })),
    chunk('IEND', Buffer.alloc(0)),
  ])
}

function inRoundedRect(x, y, x0, y0, x1, y1, r) {
  if (x < x0 || x > x1 || y < y0 || y > y1) return false
  const cx = Math.min(Math.max(x, x0 + r), x1 - r)
  const cy = Math.min(Math.max(y, y0 + r), y1 - r)
  return (x - cx) ** 2 + (y - cy) ** 2 <= r * r
}

function inRect(x, y, x0, y0, x1, y1) {
  return x >= x0 && x <= x1 && y >= y0 && y <= y1
}

/** Returns 0 (transparent), 1 (background) or 2 (white mark) for a point in [0,1]². */
function shade(x, y) {
  if (!inRoundedRect(x, y, 0.03, 0.03, 0.97, 0.97, 0.22)) return 0
  const t = 0.1
  const l = 0.25
  const a = 0.2
  const b = 1 - a
  const corners = [
    [a, a, 1, 1], [b, a, -1, 1], [a, b, 1, -1], [b, b, -1, -1],
  ]
  for (const [cx, cy, dx, dy] of corners) {
    const hx0 = Math.min(cx, cx + dx * l), hx1 = Math.max(cx, cx + dx * l)
    const hy0 = Math.min(cy, cy + dy * t), hy1 = Math.max(cy, cy + dy * t)
    const vx0 = Math.min(cx, cx + dx * t), vx1 = Math.max(cx, cx + dx * t)
    const vy0 = Math.min(cy, cy + dy * l), vy1 = Math.max(cy, cy + dy * l)
    if (inRect(x, y, hx0, hy0, hx1, hy1) || inRect(x, y, vx0, vy0, vx1, vy1)) return 2
  }
  if ((x - 0.5) ** 2 + (y - 0.5) ** 2 <= 0.12 ** 2) return 2
  return 1
}

export function renderIcon(size) {
  const ss = 4
  const out = Buffer.alloc(size * size * 4)
  const bg = [15, 118, 110]
  for (let py = 0; py < size; py++) {
    for (let px = 0; px < size; px++) {
      let cover = 0
      let white = 0
      for (let sy = 0; sy < ss; sy++) {
        for (let sx = 0; sx < ss; sx++) {
          const v = shade((px + (sx + 0.5) / ss) / size, (py + (sy + 0.5) / ss) / size)
          if (v > 0) cover++
          if (v === 2) white++
        }
      }
      const i = (py * size + px) * 4
      const alpha = cover / (ss * ss)
      const w = cover ? white / cover : 0
      out[i] = Math.round(bg[0] + (255 - bg[0]) * w)
      out[i + 1] = Math.round(bg[1] + (255 - bg[1]) * w)
      out[i + 2] = Math.round(bg[2] + (255 - bg[2]) * w)
      out[i + 3] = Math.round(alpha * 255)
    }
  }
  return encodePng(size, out)
}

export async function writeIcons(dir) {
  await mkdir(dir, { recursive: true })
  for (const size of [16, 32, 48, 128]) await writeFile(path.join(dir, `icon-${size}.png`), renderIcon(size))
}
