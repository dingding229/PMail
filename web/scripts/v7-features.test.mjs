import assert from 'node:assert/strict'
import path from 'node:path'
import test from 'node:test'
import { build } from 'esbuild'
import { Window } from 'happy-dom'

const root = path.resolve(import.meta.dirname, '..')
const win = new Window({
  url: 'http://localhost:5173/',
  settings: { disableJavaScriptFileLoading: true, disableCSSFileLoading: true, disableIframePageLoading: true },
})
for (const name of [
  'document',
  'HTMLElement',
  'Element',
  'Node',
  'NodeFilter',
  'HTMLInputElement',
  'MutationObserver',
  'getComputedStyle',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'Event',
  'CustomEvent',
  'MouseEvent',
  'KeyboardEvent',
]) {
  if (win[name]) {
    globalThis[name] =
      typeof win[name] === 'function' && !/^[A-Z]/.test(name) ? win[name].bind(win) : win[name]
  }
}
globalThis.window = win
globalThis.IS_REACT_ACT_ENVIRONMENT = true
test.after(async () => { await win.happyDOM.close() })

const bundle = await build({
  stdin: {
    resolveDir: root,
    loader: 'tsx',
    contents: `
      import { act } from 'react'
      import { createRoot } from 'react-dom/client'
      import { renderToStaticMarkup } from 'react-dom/server'
      import './src/i18n'
      import { MailTranslationCard } from './src/features/mail/MailTranslationCard'
      import { MailAIPanel } from './src/features/mail/MailAIPanel'
      import {
        buildProxyInput,
        createEmptyProxyDraft,
        validateProxyDraft,
      } from './src/components/settings/proxyHelpers'
      import { computePurgeDaysLeft } from './src/features/mail/trashHelpers'
      import { EmailListPane } from './src/features/mail/EmailListPane'

      export {
        act,
        createRoot,
        renderToStaticMarkup,
        MailTranslationCard,
        MailAIPanel,
        buildProxyInput,
        createEmptyProxyDraft,
        validateProxyDraft,
        computePurgeDaysLeft,
        EmailListPane,
      }
    `,
  },
  bundle: true,
  format: 'esm',
  platform: 'node',
  jsx: 'automatic',
  write: false,
  alias: { '@': path.join(root, 'src') },
  plugins: [{
    name: 'native-react-act',
    setup(plugin) {
      plugin.onResolve({ filter: /^react$/ }, () => ({ path: 'react', namespace: 'native-react' }))
      plugin.onLoad({ filter: /.*/, namespace: 'native-react' }, () => ({
        contents: `const { createRequire } = require('node:module'); module.exports = createRequire(${JSON.stringify(import.meta.url)})('react');`,
        loader: 'js',
      }))
    },
  }],
  banner: { js: `import { createRequire } from 'node:module'; const require = createRequire(${JSON.stringify(import.meta.url)});` },
})

const mod = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString('base64')}`)
const React = (await import('node:module')).createRequire(import.meta.url)('react')

test('translation card and AI panel render <script> and <img onerror> strictly as plain text', () => {
  const xssPayload = '<script>alert("xss")</script><img src=x onerror=alert(1)>'

  const translationHtml = mod.renderToStaticMarkup(
    React.createElement(mod.MailTranslationCard, {
      result: {
        subject: `Subject ${xssPayload}`,
        text: `Body ${xssPayload}`,
        target_lang: 'zh-CN',
        provider: 'google_web',
        engine: 'web',
        cached: false,
        truncated: false,
      },
      loading: false,
      error: null,
      showTranslation: true,
      onToggleShow: () => {},
      onRetranslate: () => {},
      onClose: () => {},
    }),
  )

  assert.doesNotMatch(translationHtml, /<script[\s>]/i)
  assert.doesNotMatch(translationHtml, /<img[\s>]/i)
  assert.match(translationHtml, /&lt;script&gt;alert\(&quot;xss&quot;\)&lt;\/script&gt;/)
  assert.match(translationHtml, /&lt;img src=x onerror=alert\(1\)&gt;/)

  const aiHtml = mod.renderToStaticMarkup(
    React.createElement(mod.MailAIPanel, {
      summary: {
        action: 'summary',
        lang: 'zh-CN',
        model: 'gpt-4o-mini',
        cached: false,
        truncated: false,
        summary: `Summary ${xssPayload}`,
        key_points: [`Point ${xssPayload}`],
      },
      summaryLoading: false,
      summaryError: null,
      extract: {
        action: 'extract',
        lang: 'zh-CN',
        model: 'gpt-4o-mini',
        cached: false,
        truncated: false,
        action_items: [{ text: `Todo ${xssPayload}`, due: '<img src=x onerror=1>' }],
        dates: [],
        amounts: [],
        links: [{ text: xssPayload, url: 'javascript:alert(1)' }],
        codes: [{ type: 'verification', value: xssPayload }],
        contacts: [],
      },
      extractLoading: false,
      extractError: null,
      onRefreshSummary: () => {},
      onRefreshExtract: () => {},
      onCloseSummary: () => {},
      onCloseExtract: () => {},
    }),
  )

  assert.doesNotMatch(aiHtml, /<script[\s>]/i)
  assert.doesNotMatch(aiHtml, /<img[\s>]/i)
  assert.doesNotMatch(aiHtml, /href="javascript:/i)
  assert.match(aiHtml, /&lt;script&gt;alert\(&quot;xss&quot;\)&lt;\/script&gt;/)
})

test('proxy fields validation enforces host, port range, and no URL prefix', () => {
  const empty = mod.createEmptyProxyDraft()
  assert.deepEqual(mod.validateProxyDraft(empty), { host: 'required', port: 'invalid' })
  assert.deepEqual(mod.validateProxyDraft(empty, { allowEmpty: true }), {})

  assert.equal(
    mod.validateProxyDraft({ ...empty, host: 'http://127.0.0.1', port: '1080' }).host,
    'invalidUrl',
  )
  assert.equal(
    mod.validateProxyDraft({ ...empty, host: '127.0.0.1', port: '0' }).port,
    'invalid',
  )
  assert.equal(
    mod.validateProxyDraft({ ...empty, host: '127.0.0.1', port: '65536' }).port,
    'invalid',
  )
  assert.equal(
    mod.validateProxyDraft({ ...empty, host: '127.0.0.1', port: '1080.5' }).port,
    'invalid',
  )

  const valid = {
    type: 'socks5',
    host: '2001:db8::10',
    port: '1080',
    username: 'operator',
    password: '',
    passwordSet: true,
    clearPassword: false,
  }
  assert.deepEqual(mod.validateProxyDraft(valid), {})
  assert.deepEqual(mod.buildProxyInput(valid), {
    type: 'socks5',
    host: '2001:db8::10',
    port: 1080,
    username: 'operator',
  })
  assert.deepEqual(mod.buildProxyInput({ ...valid, clearPassword: true }), {
    type: 'socks5',
    host: '2001:db8::10',
    port: 1080,
    username: 'operator',
    password: '',
  })
})

test('trash restore and empty trash require confirmation before permanent deletion', async (t) => {
  const host = win.document.createElement('div')
  win.document.body.append(host)
  const rootNode = mod.createRoot(host)
  t.after(async () => {
    await mod.act(async () => rootNode.unmount())
    host.remove()
  })

  const restoredCalls = []
  let emptyCalls = 0
  const now = Date.now()
  const trashEmail = {
    id: 112,
    title: 'Old CI build',
    desc: 'Archived artifact',
    datetime: '2026-11-20 10:00:00',
    datetime_iso: new Date(now - 3 * 86400_000).toISOString(),
    is_read: true,
    sender: { Name: 'CI Bot', EmailAddress: 'ci@pmail.dev' },
    to: [],
    dangerous: false,
    error: '',
    deleted_at: now - 3 * 86400_000,
    purge_at: now + 27 * 86400_000,
  }

  assert.equal(mod.computePurgeDaysLeft(trashEmail.purge_at, now), 27)
  assert.equal(mod.computePurgeDaysLeft(null, now), null)

  await mod.act(async () => {
    rootNode.render(
      React.createElement(mod.EmailListPane, {
        folder: { kind: 'system', key: 'trash' },
        folderTitle: '已删除',
        emails: [trashEmail],
        totalCount: 1,
        localDrafts: [],
        activeEmailId: 112,
        checkedIds: new Set([112]),
        searchQuery: '',
        filterMode: 'all',
        density: 'comfortable',
        isLoading: false,
        isFetching: false,
        isFetchingNextPage: false,
        hasNextPage: false,
        isError: false,
        groupOptions: [],
        isMobile: false,
        isTablet: false,
        searchInputRef: { current: null },
        onSearchChange: () => {},
        onFilterChange: () => {},
        onRefresh: () => {},
        onLoadMore: () => {},
        onSelectEmail: () => {},
        onToggleCheck: () => {},
        onSelectAll: () => {},
        onClearSelection: () => {},
        onToggleStar: () => {},
        onBulkMarkRead: () => {},
        onBulkStar: () => {},
        onBulkDelete: () => {},
        onBulkRestore: (ids) => restoredCalls.push(ids),
        onEmptyTrash: () => { emptyCalls++ },
        trashRetentionDays: 30,
        onBulkMove: () => {},
        onBulkMarkSpam: () => {},
        onPrefetchEmail: () => {},
        onCancelPrefetch: () => {},
        onResumeDraft: () => {},
        onDiscardDraft: () => {},
      }),
    )
  })

  // Restore button triggers restore immediately for selected items
  const restoreBtn = [...host.querySelectorAll('button')].find((b) =>
    b.textContent?.includes('还原'),
  )
  assert.ok(restoreBtn, 'missing restore button in Trash bulk bar')
  await mod.act(async () => restoreBtn.click())
  assert.deepEqual(restoredCalls, [[112]])

  // Empty Trash button must open confirmation dialog first, NOT empty immediately
  const emptyBtn = [...host.querySelectorAll('button')].find((b) =>
    b.textContent?.includes('清空回收站'),
  )
  assert.ok(emptyBtn, 'missing empty trash button in Trash banner')
  await mod.act(async () => emptyBtn.click())
  assert.equal(emptyCalls, 0, 'emptyTrash should wait for confirmation')

  const confirmBtn = [...win.document.querySelectorAll('button')].find((b) =>
    b.textContent?.trim() === '确认清空',
  )
  assert.ok(confirmBtn, 'missing confirm empty button in dialog')
  await mod.act(async () => confirmBtn.click())
  assert.equal(emptyCalls, 1)
})
