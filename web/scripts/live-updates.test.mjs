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
      import './src/i18n'
      import {
        BACKOFF_SCHEDULE_MS,
        HIDDEN_POLL_INTERVAL_MS,
        createLiveUpdatesLoop,
        decideNewMailPresentation,
        getBackoffDelayMs,
      } from './src/hooks/liveUpdatesCore'
      import { EmailListPane } from './src/features/mail/EmailListPane'

      export {
        act,
        createRoot,
        BACKOFF_SCHEDULE_MS,
        HIDDEN_POLL_INTERVAL_MS,
        createLiveUpdatesLoop,
        decideNewMailPresentation,
        getBackoffDelayMs,
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

function createFakeScheduler() {
  let nextId = 1
  const pending = new Map()
  return {
    scheduledDelays: [],
    scheduleTimer(ms, cb) {
      const id = nextId++
      this.scheduledDelays.push(ms)
      pending.set(id, { ms, cb })
      return id
    },
    clearTimer(id) {
      pending.delete(id)
    },
    pendingCount() {
      return pending.size
    },
    async flushNext() {
      const first = pending.entries().next().value
      if (!first) return false
      const [id, entry] = first
      pending.delete(id)
      entry.cb()
      await Promise.resolve()
      await Promise.resolve()
      return true
    },
  }
}

test('backoff sequence follows 2s, 5s, 10s, 30s, 30s and resets after success', async () => {
  assert.deepEqual(
    [1, 2, 3, 4, 5, 6].map((n) => mod.getBackoffDelayMs(n)),
    [2000, 5000, 10000, 30000, 30000, 30000],
  )

  const scheduler = createFakeScheduler()
  let callCount = 0
  const loop = mod.createLiveUpdatesLoop({
    fetchChanges: async () => {
      callCount += 1
      if (callCount <= 5) {
        const err = new Error('live_updates_busy')
        err.status = 503
        throw err
      }
      if (callCount === 6) {
        return { cursor: 'c6', changed: false, new_count: 0, new_items: [] }
      }
      const err = new Error('network error')
      err.status = 500
      throw err
    },
    isDocumentHidden: () => false,
    onChanges: () => {},
    scheduleTimer: scheduler.scheduleTimer.bind(scheduler),
    clearTimer: scheduler.clearTimer.bind(scheduler),
  })

  loop.start()
  for (let i = 0; i < 7; i += 1) {
    await Promise.resolve()
    await Promise.resolve()
    if (i < 6) await scheduler.flushNext()
  }
  loop.stop()

  // 5 errors -> 2000, 5000, 10000, 30000, 30000; 6th succeeds -> 0; 7th fails -> reset to 2000
  assert.deepEqual(scheduler.scheduledDelays, [2000, 5000, 10000, 30000, 30000, 0, 2000])
})

test('cursor starts empty and propagates returned cursor across requests', async () => {
  const scheduler = createFakeScheduler()
  const receivedCursors = []
  const delivered = []

  const loop = mod.createLiveUpdatesLoop({
    fetchChanges: async ({ cursor }) => {
      receivedCursors.push(cursor)
      if (receivedCursors.length === 1) {
        return { cursor: 'cur-1', changed: false, new_count: 0, new_items: [] }
      }
      if (receivedCursors.length === 2) {
        return {
          cursor: 'cur-2',
          changed: true,
          new_count: 1,
          new_items: [{ id: 501, subject: 'Hello', from_name: 'Alice' }],
        }
      }
      return { cursor: 'cur-3', changed: false, new_count: 0, new_items: [] }
    },
    isDocumentHidden: () => false,
    onChanges: (res) => delivered.push(res),
    scheduleTimer: scheduler.scheduleTimer.bind(scheduler),
    clearTimer: scheduler.clearTimer.bind(scheduler),
  })

  loop.start()
  await Promise.resolve()
  await Promise.resolve()
  await scheduler.flushNext()
  await scheduler.flushNext()
  loop.stop()

  assert.deepEqual(receivedCursors, ['', 'cur-1', 'cur-2'])
  assert.equal(delivered.length, 1)
  assert.equal(delivered[0].new_items[0].id, 501)
})

test('hidden page reduces polling to 60s wait=0 and resumes immediately on visible', async () => {
  const scheduler = createFakeScheduler()
  let hidden = false
  const calls = []

  const loop = mod.createLiveUpdatesLoop({
    fetchChanges: async ({ cursor, wait }) => {
      calls.push({ cursor, wait })
      return { cursor: `c${calls.length}`, changed: false, new_count: 0, new_items: [] }
    },
    isDocumentHidden: () => hidden,
    onChanges: () => {},
    scheduleTimer: scheduler.scheduleTimer.bind(scheduler),
    clearTimer: scheduler.clearTimer.bind(scheduler),
  })

  loop.start()
  await Promise.resolve()
  await Promise.resolve()
  // Call 1 while visible -> wait=25, schedules immediate (0ms)
  assert.deepEqual(calls[0], { cursor: '', wait: 25 })

  // Hide document before next poll fires
  hidden = true
  await scheduler.flushNext()
  // Call 2 while hidden -> wait=0, schedules 60s sleep
  assert.deepEqual(calls[1], { cursor: 'c1', wait: 0 })
  assert.equal(scheduler.scheduledDelays[scheduler.scheduledDelays.length - 1], 60_000)

  // Become visible again -> cancels 60s sleep and polls immediately with wait=25
  hidden = false
  loop.handleVisibilityChange()
  await Promise.resolve()
  await Promise.resolve()
  assert.deepEqual(calls[2], { cursor: 'c2', wait: 25 })
  loop.stop()
})

test('scrolled list or active selection shows new-mail pill without disrupting list', async (t) => {
  // Pure decision matrix
  assert.deepEqual(
    mod.decideNewMailPresentation({
      isScrolledDown: false,
      hasMultiSelection: false,
      isInbox: true,
      newCount: 1,
    }),
    { shouldAutoRefreshList: true, shouldShowPill: false, shouldShowToast: false },
  )
  assert.deepEqual(
    mod.decideNewMailPresentation({
      isScrolledDown: true,
      hasMultiSelection: false,
      isInbox: true,
      newCount: 2,
    }),
    { shouldAutoRefreshList: false, shouldShowPill: true, shouldShowToast: true },
  )
  assert.deepEqual(
    mod.decideNewMailPresentation({
      isScrolledDown: false,
      hasMultiSelection: true,
      isInbox: true,
      newCount: 1,
    }),
    { shouldAutoRefreshList: false, shouldShowPill: true, shouldShowToast: true },
  )

  // Component verification: EmailListPane renders the floating pill and triggers refresh callback
  const host = win.document.createElement('div')
  win.document.body.append(host)
  const rootNode = mod.createRoot(host)
  t.after(async () => {
    await mod.act(async () => rootNode.unmount())
    host.remove()
  })

  let pillClicks = 0
  await mod.act(async () => {
    rootNode.render(
      React.createElement(mod.EmailListPane, {
        folder: { kind: 'system', key: 'inbox' },
        folderTitle: '收件箱',
        emails: [],
        totalCount: 0,
        localDrafts: [],
        activeEmailId: null,
        checkedIds: new Set(),
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
        onBulkMove: () => {},
        onBulkMarkSpam: () => {},
        onPrefetchEmail: () => {},
        onCancelPrefetch: () => {},
        onResumeDraft: () => {},
        onDiscardDraft: () => {},
        pendingNewCount: 3,
        onShowPendingNew: () => { pillClicks += 1 },
      }),
    )
  })

  const pillBtn = [...host.querySelectorAll('button')].find((b) =>
    b.textContent?.includes('3 封新邮件'),
  )
  assert.ok(pillBtn, 'expected floating new-mail pill button')
  await mod.act(async () => pillBtn.click())
  assert.equal(pillClicks, 1)
})

test('HTTP 401 stops the live-update loop immediately without scheduling retries', async () => {
  const scheduler = createFakeScheduler()
  let unauthorizedCalls = 0
  let fetchCalls = 0

  const loop = mod.createLiveUpdatesLoop({
    fetchChanges: async () => {
      fetchCalls += 1
      const err = new Error('Unauthorized')
      err.status = 401
      throw err
    },
    isDocumentHidden: () => false,
    onChanges: () => {},
    onUnauthorized: () => { unauthorizedCalls += 1 },
    scheduleTimer: scheduler.scheduleTimer.bind(scheduler),
    clearTimer: scheduler.clearTimer.bind(scheduler),
  })

  loop.start()
  await Promise.resolve()
  await Promise.resolve()

  assert.equal(fetchCalls, 1)
  assert.equal(unauthorizedCalls, 1)
  assert.equal(loop.isStopped(), true)
  assert.equal(scheduler.pendingCount(), 0)
  assert.deepEqual(scheduler.scheduledDelays, [])
})
