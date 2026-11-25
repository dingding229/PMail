import assert from 'node:assert/strict'
import path from 'node:path'
import test from 'node:test'
import { build } from 'esbuild'
import { Window } from 'happy-dom'

const root = path.resolve(import.meta.dirname, '..')
const win = new Window({ url: 'https://mail.example/' })
for (const k of ['window', 'document', 'DOMParser', 'Node', 'Element', 'HTMLElement']) {
  globalThis[k] = k === 'window' ? win : win[k] ?? win.document
}
globalThis.document = win.document

const bundle = await build({
  stdin: {
    resolveDir: root,
    loader: 'tsx',
    contents: `
      export { processEmailHtml, stripTrackingImages } from './src/features/mail/emailHtml'
      export { proxiedImageUrl } from './src/features/mail/remoteImages'
    `,
  },
  bundle: true,
  format: 'esm',
  platform: 'node',
  jsx: 'automatic',
  write: false,
  alias: { '@': path.join(root, 'src') },
  banner: { js: `import { createRequire } from 'node:module'; const require = createRequire(${JSON.stringify(import.meta.url)});` },
})
const mod = await import(
  'data:text/javascript;base64,' + Buffer.from(bundle.outputFiles[0].text).toString('base64')
)
const { processEmailHtml, proxiedImageUrl } = mod

const mail = `
  <style>.hero{background:url(https://cdn.sender.example/hero.png)}</style>
  <table background="https://cdn.sender.example/bg.jpg"><tr><td>
    <img src="https://cdn.sender.example/logo.png" width="120" height="40" alt="Logo">
    <img src="//cdn.sender.example/rel.png" srcset="https://cdn.sender.example/a.png 1x, https://cdn.sender.example/b.png 2x">
    <img src="https://track.example/open?id=42" width="1" height="1">
    <img src="https://track.example/hidden.gif" style="display:none">
    <div style="background-image:url('https://cdn.sender.example/s.png')">x</div>
    <video poster="https://track.example/poster.jpg" src="https://track.example/v.mp4"></video>
    <img src="data:image/png;base64,iVBORw0KGgo=">
  </td></tr></table>`

const externalHosts = /(?:sender|track)\.example/

function attrs(html) {
  const doc = new win.DOMParser().parseFromString(html, 'text/html')
  const urls = []
  doc.querySelectorAll('[src],[srcset],[background],[poster]').forEach((el) => {
    for (const a of ['src', 'srcset', 'background', 'poster']) {
      const v = el.getAttribute(a)
      if (v) urls.push(v)
    }
  })
  doc.querySelectorAll('[style],style').forEach((el) =>
    urls.push(el.getAttribute('style') || el.textContent || ''),
  )
  return urls
}

test('blocked: no URL in the rendered mail can reach the sender', () => {
  const out = processEmailHtml(mail, 7, 'tok', false, 'Image blocked', 'notice')
  assert.equal(out.hasExternalImages, true)
  for (const u of attrs(out.bodyHtml)) assert.doesNotMatch(u, externalHosts, u)
  assert.doesNotMatch(out.bodyHtml, /track\.example/)
})

test('allowed: every external image goes through the proxy, pixels are dropped', () => {
  const out = processEmailHtml(mail, 7, 'tok', true, 'Image blocked', 'notice')
  const urls = attrs(out.bodyHtml)
  for (const u of urls) {
    if (/^data:/.test(u)) continue
    // Each URL either is a proxy URL or contains the sender host only encoded inside one.
    for (const piece of u.split(/[\s,()'"]+/).filter((p) => /example/.test(p))) {
      assert.match(piece, /^\/api\/email\/image\/7\?t=tok&u=https%3A%2F%2F/, piece)
    }
  }
  assert.doesNotMatch(out.bodyHtml, /track\.example/, 'tracking pixel or media survived')
  assert.match(out.bodyHtml, /u=https%3A%2F%2Fcdn\.sender\.example%2Frel\.png/, 'protocol-relative URL')
  assert.match(out.bodyHtml, /referrerpolicy="no-referrer"/)
})

test('without a proxy token images stay blocked even when allowed', () => {
  const out = processEmailHtml(mail, 7, '', true, 'Image blocked', 'notice')
  for (const u of attrs(out.bodyHtml)) assert.doesNotMatch(u, externalHosts, u)
})

test('proxy URL encodes the original', () => {
  assert.equal(
    proxiedImageUrl(3, 'a b', '//x.example/p.png?a=1&b=2'),
    '/api/email/image/3?t=a%20b&u=https%3A%2F%2Fx.example%2Fp.png%3Fa%3D1%26b%3D2',
  )
})

test('quoted HTML loses tracking pixels but keeps real images', async () => {
  const { stripTrackingImages } = mod
  const out = stripTrackingImages(
    '<img src="https://t.example/p" width="1" height="1"><img src="https://t.example/h" style="display:none"><img src="https://cdn.example/a.png" width="200">',
  )
  assert.doesNotMatch(out, /t\.example/)
  assert.match(out, /cdn\.example\/a\.png/)
})

test('kept <style> sheets survive and never point at relative URLs', () => {
  const html =
    '<p class="hero">Hi</p><div style="background:url(/api/email/del)">x</div>' +
    '<style>.hero{color:#f60;background:url("https://cdn.example/h.png")} .bad{background:url(/api/x)}</style>'
  const blocked = processEmailHtml(html, 7, 'tok', false, 'img', 'blocked')
  assert.match(blocked.bodyHtml, /<style>[^<]*\.hero\{color:#f60;background:none\}/)
  assert.doesNotMatch(blocked.bodyHtml, /\/api\//)
  assert.equal(blocked.hasExternalImages, true)
  const allowed = processEmailHtml(html, 7, 'tok', true, 'img', 'blocked')
  assert.match(allowed.bodyHtml, /url\("\/api\/email\/image\/7\?t=tok&amp;u=https%3A%2F%2Fcdn\.example%2Fh\.png"\)|url\("\/api\/email\/image\/7\?t=tok&u=https%3A%2F%2Fcdn\.example%2Fh\.png"\)/)
  assert.doesNotMatch(allowed.bodyHtml, /url\(\/api\/x\)|url\(\/api\/email\/del\)/)
})
