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
  'HTMLFormElement',
  'HTMLSelectElement',
  'HTMLButtonElement',
  'HTMLDivElement',
  'DocumentFragment',
  'SVGElement',
  'Text',
  'Comment',
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
      import { MemoryRouter } from 'react-router'
      import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
      import './src/i18n'
      import { MailReportsSection } from './src/features/admin/MailReportsSection'
      import { computePassRate } from './src/components/charts/reportUtils'
      import { ReportMailSection } from './src/features/settings/sections/ReportMailSection'

      export {
        act,
        createRoot,
        MemoryRouter,
        QueryClient,
        QueryClientProvider,
        MailReportsSection,
        computePassRate,
        ReportMailSection,
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

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

async function flushUi(check, timeoutMs = 1500) {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    await mod.act(async () => {
      await new Promise((r) => setTimeout(r, 20))
    })
    if (check()) return
  }
}

test('computePassRate calculates percentages accurately and MailReportsSection highlights <98% rates and suspicious IPs', async (t) => {
  assert.equal(mod.computePassRate(99, 100), 99)
  assert.equal(mod.computePassRate(95, 100), 95)
  assert.equal(mod.computePassRate(0, 0), 0)
  assert.equal(mod.computePassRate(50, 100, 97.5), 97.5)

  const prevFetch = globalThis.fetch
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.includes('/api/domain/list')) {
      return jsonResponse({
        errorNo: 0,
        errorMsg: '',
        data: [{ id: 1, name: 'pmail.dev', is_default: true, dkim_selector: 'default', created: '', updated: '' }],
      })
    }
    if (u.includes('/api/admin/mail-reports/summary')) {
      return jsonResponse({
        errorNo: 0,
        errorMsg: '',
        data: {
          domain: 'pmail.dev',
          from: '2026-10-26',
          to: '2026-11-24',
          report_count: 10,
          latest_report_at: '2026-11-24',
          latest_policy: { p: 'none', pct: 100, report_date: '2026-11-24' },
          totals: {
            total: 200,
            dmarc_pass: 198, // 99.0% (>= 98%, no warning)
            spf_pass: 190,   // 95.0% (< 98%, warning!)
            dkim_pass: 197,  // 98.5% (>= 98%, no warning)
            quarantined: 0,
            rejected: 0,
          },
          daily: [
            { date: '2026-11-23', pass: 98, fail: 2 },
            { date: '2026-11-24', pass: 100, fail: 0 },
          ],
          reporters: [{ org_name: 'Google', count: 200, pass_rate: 99.0 }],
          sources: [
            { source_ip: '203.0.113.10', count: 198, pass_rate: 100, is_own: true, suspicious: false },
            { source_ip: '185.220.101.42', count: 2, pass_rate: 0, is_own: false, suspicious: true },
          ],
          tls: null,
        },
      })
    }
    if (u.includes('/api/admin/mail-reports/records')) {
      return jsonResponse({ errorNo: 0, errorMsg: '', data: { items: [], total: 0, page: 1, page_size: 10 } })
    }
    return jsonResponse({ errorNo: 0, errorMsg: '', data: null })
  }
  t.after(() => { globalThis.fetch = prevFetch })

  const host = win.document.createElement('div')
  win.document.body.append(host)
  const qc = new mod.QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { gcTime: 0 } } })
  const rootNode = mod.createRoot(host)
  t.after(async () => {
    await mod.act(async () => rootNode.unmount())
    qc.clear()
    host.remove()
  })

  await mod.act(async () => {
    rootNode.render(
      React.createElement(
        mod.MemoryRouter,
        null,
        React.createElement(
          mod.QueryClientProvider,
          { client: qc },
          React.createElement(mod.MailReportsSection),
        ),
      ),
    )
    await new Promise((r) => setTimeout(r, 40))
  })

  await flushUi(() => Boolean(host.querySelector('[data-kpi="dmarc"]')))

  const dmarcCard = host.querySelector('[data-kpi="dmarc"]')
  const spfCard = host.querySelector('[data-kpi="spf"]')
  const dkimCard = host.querySelector('[data-kpi="dkim"]')
  assert.ok(dmarcCard && spfCard && dkimCard, 'KPI cards should render')
  assert.match(dmarcCard.textContent || '', /99\.0%/)
  assert.equal(dmarcCard.getAttribute('data-warning'), 'false')
  assert.match(spfCard.textContent || '', /95\.0%/)
  assert.equal(spfCard.getAttribute('data-warning'), 'true', 'SPF < 98% should be marked warning')
  assert.match(dkimCard.textContent || '', /98\.5%/)
  assert.equal(dkimCard.getAttribute('data-warning'), 'false')

  // Suspicious IP highlighting + explanation
  const suspiciousNodes = host.querySelectorAll('[data-source-ip="185.220.101.42"][data-suspicious="true"]')
  assert.ok(suspiciousNodes.length > 0, 'suspicious IP row should be highlighted')
  assert.match(host.textContent || '', /通常是有人冒充/)
  assert.match(host.textContent || '', /暂无 TLS 报告/)
  // Charts have role="img" and descriptive aria-label
  const charts = host.querySelectorAll('svg[role="img"]')
  assert.ok(charts.length >= 2, 'charts should have role="img"')
  for (const chart of charts) {
    assert.ok((chart.getAttribute('aria-label') || '').length > 5)
  }
})

test('MailReportsSection shows empty state explaining DMARC rua= and 1-day delay', async (t) => {
  const prevFetch = globalThis.fetch
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.includes('/api/domain/list')) {
      return jsonResponse({
        errorNo: 0,
        errorMsg: '',
        data: [{ id: 2, name: 'studio.cn', is_default: true, dkim_selector: 'default', created: '', updated: '' }],
      })
    }
    if (u.includes('/api/admin/mail-reports/summary')) {
      return jsonResponse({
        errorNo: 0,
        errorMsg: '',
        data: {
          domain: 'studio.cn',
          from: '2026-10-26',
          to: '2026-11-24',
          report_count: 0,
          latest_policy: null,
          totals: { total: 0, dmarc_pass: 0, spf_pass: 0, dkim_pass: 0, quarantined: 0, rejected: 0 },
          daily: [],
          reporters: [],
          sources: [],
          tls: null,
        },
      })
    }
    return jsonResponse({ errorNo: 0, errorMsg: '', data: null })
  }
  t.after(() => { globalThis.fetch = prevFetch })

  const host = win.document.createElement('div')
  win.document.body.append(host)
  const qc = new mod.QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { gcTime: 0 } } })
  const rootNode = mod.createRoot(host)
  t.after(async () => {
    await mod.act(async () => rootNode.unmount())
    qc.clear()
    host.remove()
  })

  await mod.act(async () => {
    rootNode.render(
      React.createElement(
        mod.MemoryRouter,
        null,
        React.createElement(
          mod.QueryClientProvider,
          { client: qc },
          React.createElement(mod.MailReportsSection),
        ),
      ),
    )
    await new Promise((r) => setTimeout(r, 40))
  })

  await flushUi(() => (host.textContent || '').includes('暂无邮件报告数据'))

  const text = host.textContent || ''
  assert.match(text, /暂无邮件报告数据/)
  assert.match(text, /rua=/)
  assert.match(text, /一天/)
})

test('ReportMailSection saves preference switches and triggers apply-existing action', async (t) => {
  const prevFetch = globalThis.fetch
  const savedPayloads = []
  let applyCalls = 0
  let currentPref = { enabled: true, group_id: 0, mark_read: true }

  globalThis.fetch = async (url, init) => {
    const u = String(url)
    const method = (init?.method || 'GET').toUpperCase()
    if (u.includes('/api/group/list')) {
      return jsonResponse({ errorNo: 0, errorMsg: '', data: [] })
    }
    if (u.endsWith('/api/settings/report-mail') && method === 'GET') {
      return jsonResponse({ errorNo: 0, errorMsg: '', data: currentPref })
    }
    if (u.endsWith('/api/settings/report-mail') && method === 'PUT') {
      const body = JSON.parse(String(init?.body || '{}'))
      savedPayloads.push(body)
      currentPref = { ...currentPref, ...body }
      return jsonResponse({ errorNo: 0, errorMsg: '', data: currentPref })
    }
    if (u.endsWith('/api/settings/report-mail/apply') && method === 'POST') {
      applyCalls += 1
      return jsonResponse({ errorNo: 0, errorMsg: '', data: { moved: 6, group_id: 21 } })
    }
    return jsonResponse({ errorNo: 0, errorMsg: '', data: null })
  }
  t.after(() => { globalThis.fetch = prevFetch })

  const host = win.document.createElement('div')
  win.document.body.append(host)
  const qc = new mod.QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { gcTime: 0 } } })
  const rootNode = mod.createRoot(host)
  t.after(async () => {
    await mod.act(async () => rootNode.unmount())
    qc.clear()
    host.remove()
  })

  await mod.act(async () => {
    rootNode.render(
      React.createElement(
        mod.QueryClientProvider,
        { client: qc },
        React.createElement(mod.ReportMailSection),
      ),
    )
    await new Promise((r) => setTimeout(r, 40))
  })

  await flushUi(() => Boolean(host.querySelector('#report_mail_mark_read')))

  const markReadSwitch = host.querySelector('#report_mail_mark_read')
  assert.ok(markReadSwitch, 'mark_read switch should render')
  await mod.act(async () => {
    markReadSwitch.click()
    await new Promise((r) => setTimeout(r, 30))
  })
  assert.equal(savedPayloads.length, 1)
  assert.equal(savedPayloads[0].mark_read, false)

  const enabledSwitch = host.querySelector('#report_mail_enabled')
  assert.ok(enabledSwitch, 'enabled switch should render')
  await mod.act(async () => {
    enabledSwitch.click()
    await new Promise((r) => setTimeout(r, 30))
  })
  assert.equal(savedPayloads.length, 2)
  assert.equal(savedPayloads[1].enabled, false)

  const applyBtn = Array.from(host.querySelectorAll('button')).find((b) =>
    (b.textContent || '').includes('归类现有邮件'),
  )
  assert.ok(applyBtn, 'apply existing button should render')
  await mod.act(async () => {
    applyBtn.click()
    await new Promise((r) => setTimeout(r, 30))
  })
  assert.equal(applyCalls, 1)
})
