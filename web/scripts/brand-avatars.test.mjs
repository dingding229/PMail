import assert from 'node:assert/strict'
import path from 'node:path'
import test from 'node:test'
import { build } from 'esbuild'

const root = path.resolve(import.meta.dirname, '..')

const bundle = await build({
  stdin: {
    resolveDir: root,
    loader: 'tsx',
    contents: `
      import { renderToStaticMarkup } from 'react-dom/server'
      import {
        BRAND_ICON_CATALOG,
        isSenderAuthVerified,
        matchBrandByEmail,
        isUnbrandableHost,
      } from './src/lib/brandAvatars'
      import { SenderAvatar } from './src/components/common/SenderAvatar'

      export {
        renderToStaticMarkup,
        BRAND_ICON_CATALOG,
        isSenderAuthVerified,
        matchBrandByEmail,
        isUnbrandableHost,
        SenderAvatar,
      }
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

const mod = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString('base64')}`)
const React = (await import('node:module')).createRequire(import.meta.url)('react')

test('brand matching supports exact domain, subdomain, and case-insensitivity, and rejects lookalikes', () => {
  // Exact & subdomain
  assert.equal(mod.matchBrandByEmail('noreply@github.com')?.id, 'github')
  assert.equal(mod.matchBrandByEmail('no_reply@weixin.qq.com')?.id, 'wechat')
  assert.equal(mod.matchBrandByEmail('x@mail.github.com')?.id, 'github')
  assert.equal(mod.matchBrandByEmail('noreply@notify.cloudflare.com')?.id, 'cloudflare')

  // Case-insensitive & angle-bracket format
  assert.equal(mod.matchBrandByEmail('NoReply@Mail.GITHUB.COM')?.id, 'github')
  assert.equal(mod.matchBrandByEmail('GitHub <NOTIFICATIONS@GITHUB.COM>')?.id, 'github')

  // Lookalike rejection
  assert.equal(mod.matchBrandByEmail('alerts@evilgithub.com'), null)
  assert.equal(mod.matchBrandByEmail('security@my-github.com'), null)
  assert.equal(mod.matchBrandByEmail('user@notcloudflare.com'), null)
})

test('anti-spoofing gate requires auth_status=pass and no medium/high risk', () => {
  assert.equal(mod.isSenderAuthVerified('pass', { level: 'none', findings: [] }), true)
  assert.equal(mod.isSenderAuthVerified('pass', { level: 'low', findings: [] }), true)
  assert.equal(mod.isSenderAuthVerified('pass', undefined), true)

  assert.equal(mod.isSenderAuthVerified('fail', { level: 'none', findings: [] }), false)
  assert.equal(mod.isSenderAuthVerified('none', { level: 'none', findings: [] }), false)
  assert.equal(mod.isSenderAuthVerified(undefined, { level: 'none', findings: [] }), false)
  assert.equal(mod.isSenderAuthVerified('pass', { level: 'medium', findings: [] }), false)
  assert.equal(mod.isSenderAuthVerified('pass', { level: 'high', findings: [] }), false)
  assert.equal(
    mod.isSenderAuthVerified('pass', {
      level: 'low',
      findings: [{ severity: 'high', code: 'x', location: 'body' }],
    }),
    false,
  )
})

test('SenderAvatar falls back to initials when unverified or preference disabled', () => {
  const verifiedHtml = mod.renderToStaticMarkup(
    React.createElement(mod.SenderAvatar, {
      name: 'GitHub',
      email: 'notifications@github.com',
      verified: true,
      showBrandAvatar: true,
    }),
  )
  assert.match(verifiedHtml, /data-brand-avatar="github"/)
  assert.match(verifiedHtml, /<svg\b/)

  const unverifiedHtml = mod.renderToStaticMarkup(
    React.createElement(mod.SenderAvatar, {
      name: 'GitHub Security',
      email: 'security@github.com',
      verified: false,
      showBrandAvatar: true,
    }),
  )
  assert.doesNotMatch(unverifiedHtml, /data-brand-avatar=/)
  assert.doesNotMatch(unverifiedHtml, /<svg\b/)

  const disabledPrefHtml = mod.renderToStaticMarkup(
    React.createElement(mod.SenderAvatar, {
      name: 'GitHub',
      email: 'notifications@github.com',
      verified: true,
      showBrandAvatar: false,
    }),
  )
  assert.doesNotMatch(disabledPrefHtml, /data-brand-avatar=/)
  assert.doesNotMatch(disabledPrefHtml, /<svg\b/)
})

test('generated brand SVGs contain no script, on*, foreignObject, external href, or colliding IDs', () => {
  assert.ok(mod.BRAND_ICON_CATALOG.length >= 140, 'expected ~150 brands in catalog')

  const seenIds = new Set()
  for (const brand of mod.BRAND_ICON_CATALOG) {
    if (!brand.svg) continue
    assert.doesNotMatch(brand.svg, /<script\b/i, `${brand.id} must not contain <script>`)
    assert.doesNotMatch(brand.svg, /<foreignObject\b/i, `${brand.id} must not contain <foreignObject>`)
    assert.doesNotMatch(brand.svg, /\son[a-z0-9_-]+\s*=/i, `${brand.id} must not contain on* handlers`)
    assert.doesNotMatch(
      brand.svg,
      /\b(?:xlink:)?href\s*=\s*["'](?!#)/i,
      `${brand.id} must not contain external href`,
    )

    // Verify all gradient/clipPath/mask IDs are globally unique across all icons
    const idMatches = brand.svg.matchAll(/\bid="([^"]+)"/g)
    for (const m of idMatches) {
      const id = m[1]
      assert.ok(
        id.startsWith(`bi-${brand.id}-`),
        `ID "${id}" in "${brand.id}" must be prefixed with "bi-${brand.id}-"`,
      )
      assert.equal(seenIds.has(id), false, `Duplicate SVG ID detected: ${id}`)
      seenIds.add(id)
    }
  }
})

test('public mailbox and shared sending domains never get a brand avatar', async () => {
  const { BRAND_REGISTRY } = await import('./brand-registry.mjs')
  for (const addr of ['someone@gmail.com', 'x@qq.com', 'x@163.com', 'x@126.com', 'x@outlook.com',
    'x@icloud.com', 'x@yahoo.com', 'x@proton.me', 'x@aliyun.com', 'x@sina.com.cn',
    'bounce@em123.sendgrid.net', 'noreply@my-app.firebaseapp.com', 'jira@evil.atlassian.net']) {
    assert.equal(mod.matchBrandByEmail(addr), null, addr)
  }
  for (const brand of BRAND_REGISTRY) {
    for (const d of brand.domains) {
      assert.equal(mod.isUnbrandableHost(d), false, `${brand.id} lists public mailbox domain ${d}`)
    }
  }
  assert.equal(mod.matchBrandByEmail('noreply@github.com')?.id, 'github')
  assert.equal(mod.matchBrandByEmail('no_reply@weixin.qq.com')?.id, 'wechat')
})
