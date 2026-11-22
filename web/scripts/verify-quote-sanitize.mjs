#!/usr/bin/env node
/**
 * Standalone assertion test for V5-U5R1:
 * Verifies reply/forward quote sanitization, paste sanitization, and signature preview
 * never leak external image/CSS/media URLs or unresolved cid: references.
 *
 * Usage: node scripts/verify-quote-sanitize.mjs
 */
import assert from 'node:assert/strict'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'esbuild'
import { Window } from 'happy-dom'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const ROOT = path.resolve(__dirname, '..')

// Set up browser-like DOM globals for DOMPurify & DOMParser
const win = new Window({ url: 'http://localhost:5173/' })
globalThis.window = win
globalThis.document = win.document
globalThis.DOMParser = win.DOMParser
globalThis.Node = win.Node
globalThis.Element = win.Element
globalThis.HTMLElement = win.HTMLElement

// Bundle paste-sanitizer.ts in-memory with esbuild so we test the exact production TypeScript source
const result = await build({
  entryPoints: [path.join(ROOT, 'src/features/mail/editor/paste-sanitizer.ts')],
  bundle: true,
  format: 'esm',
  platform: 'browser',
  write: false,
  alias: { '@': path.join(ROOT, 'src') },
})

const code = result.outputFiles[0].text
const mod = await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`)
const { sanitizePastedHtml } = mod

// 1. Reply/Forward quote with tracking pixels, CSS background:url, srcset, picture/source, video poster, cid:
const maliciousEmailHtml = `
  <div style="background: url(http://tracker.evil.example/bg1.gif); background-image: url('//tracker.evil.example/bg2.png'); color: #0f172a;" background="https://tracker.evil.example/legacy-bg.png">
    <p>原邮件正文保留 <strong>粗体</strong> 与 <pre><code class="language-go">fmt.Println("ok")</code></pre></p>
    <img src="https://tracker.evil.example/open/pixel.gif?uid=42" width="1" height="1" />
    <img src="http://tracker.evil.example/logo.png" alt="品牌横幅" srcset="https://tracker.evil.example/logo-2x.png 2x" />
    <img src="//cdn.evil.example/proto-rel.webp" />
    <picture>
      <source srcset="https://tracker.evil.example/pic.avif" type="image/avif" />
      <img src="https://tracker.evil.example/pic.jpg" alt="PictureFallback" />
    </picture>
    <video poster="https://tracker.evil.example/poster.jpg" src="https://tracker.evil.example/v.mp4"></video>
    <img src="cid:missing-inline@mail.example" alt="缺失内嵌图" />
    <img src="cid:valid-logo@mail.example" alt="系统内嵌图" />
  </div>
`

const quotedBlocked = sanitizePastedHtml(maliciousEmailHtml, {
  blockExternalImages: true,
  resolveCidUrl: (cid) => (cid === 'valid-logo@mail.example' ? '/api/attachments/101.0' : null),
  imagePlaceholder: (label) => `[图片：${label}]`,
  fallbackImageLabel: '外部图片',
})

assert.ok(!quotedBlocked.includes('evil.example'), `External host leaked into quoted HTML: ${quotedBlocked}`)
assert.ok(!quotedBlocked.includes('srcset'), `srcset leaked into quoted HTML: ${quotedBlocked}`)
assert.ok(!quotedBlocked.includes('poster'), `poster leaked into quoted HTML: ${quotedBlocked}`)
assert.ok(!quotedBlocked.includes('<picture'), `<picture> leaked into quoted HTML: ${quotedBlocked}`)
assert.ok(!quotedBlocked.includes('<source'), `<source> leaked into quoted HTML: ${quotedBlocked}`)
assert.ok(!quotedBlocked.includes('<video'), `<video> leaked into quoted HTML: ${quotedBlocked}`)
assert.ok(!quotedBlocked.includes('url('), `CSS url() leaked into quoted HTML: ${quotedBlocked}`)
assert.ok(!quotedBlocked.includes('cid:'), `Unresolved cid: leaked into quoted HTML: ${quotedBlocked}`)
assert.ok(quotedBlocked.includes('[图片：pixel.gif]'), 'Tracking pixel must become non-clickable placeholder')
assert.ok(quotedBlocked.includes('[图片：品牌横幅]'), 'HTTP image with alt must use alt in placeholder')
assert.ok(quotedBlocked.includes('[图片：proto-rel.webp]'), 'Protocol-relative // image must be blocked')
assert.ok(quotedBlocked.includes('[图片：缺失内嵌图]'), 'Unresolved cid: image must become placeholder')
assert.ok(quotedBlocked.includes('src="/api/attachments/101.0"'), 'Resolvable cid: should map to same-origin attachment URL')
assert.ok(quotedBlocked.includes('language-go'), 'Code block language class should be preserved')

// 2. When user explicitly clicked "Load external images" in ReadingPane, external <img> is allowed,
//    but CSS url(...), srcset, picture/source, video poster, and unresolved cid: are STILL stripped!
const quotedAllowed = sanitizePastedHtml(maliciousEmailHtml, {
  blockExternalImages: false,
  resolveCidUrl: (cid) => (cid === 'valid-logo@mail.example' ? '/api/attachments/101.0' : null),
  imagePlaceholder: (label) => `[图片：${label}]`,
})
assert.ok(quotedAllowed.includes('https://tracker.evil.example/open/pixel.gif?uid=42'), 'Allowed external img should remain')
assert.ok(!quotedAllowed.includes('url('), 'CSS url() must still be stripped even when external images allowed')
assert.ok(!quotedAllowed.includes('srcset'), 'srcset must still be stripped')
assert.ok(!quotedAllowed.includes('poster'), 'video poster must still be stripped')
assert.ok(!quotedAllowed.includes('cid:'), 'cid: must never remain raw')

// 3. data: image size limit & SVG data: rejection
const tinyPng = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='
const oversizedPng = `data:image/png;base64,${'A'.repeat(2048)}`
const svgData = 'data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+PC9zdmc+'
const dataHtml = `
  <img src="${tinyPng}" alt="Tiny" />
  <img src="${oversizedPng}" alt="TooBig" />
  <img src="${svgData}" alt="SvgXss" />
`
const cleanedData = sanitizePastedHtml(dataHtml, {
  blockExternalImages: true,
  maxDataImageBytes: 512,
  imagePlaceholder: (label) => `[图片：${label}]`,
})
assert.ok(cleanedData.includes(tinyPng), 'Small PNG data: image should be kept')
assert.ok(!cleanedData.includes(oversizedPng) && cleanedData.includes('[图片：TooBig]'), 'Oversized data: image must be replaced')
assert.ok(!cleanedData.includes('svg+xml') && cleanedData.includes('[图片：SvgXss]'), 'SVG data: image must be blocked')

// 4. Signature preview must NEVER load external images or CSS background URLs
const dirtySignature = `
  <p style="background: url(https://tracker.evil.example/sig-bg.png); color: #64748b;">
    林深 <img src="https://tracker.evil.example/sig-pixel.png" alt="签名图" />
  </p>
`
const cleanedSig = sanitizePastedHtml(dirtySignature, {
  blockExternalImages: true,
  imagePlaceholder: (label) => `[图片：${label}]`,
})
assert.ok(!cleanedSig.includes('evil.example'), `External host leaked into signature HTML: ${cleanedSig}`)
assert.ok(cleanedSig.includes('[图片：签名图]'), 'Signature external image must be replaced with placeholder')

console.log('✅ All 4 quote/paste/signature external-resource sanitization test suites passed (0 external URLs leaked).')
