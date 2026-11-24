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
  'FormData',
  'File',
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
      import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
      import './src/i18n'
      import { apiGet, notifyMaintenanceState } from './src/api/client'
      import { MaintenanceGate } from './src/components/common/MaintenanceGate'
      import { BackupSection } from './src/features/admin/BackupSection'
      import { MailImportExportSection } from './src/features/settings/sections/MailImportExportSection'

      export {
        act,
        createRoot,
        QueryClient,
        QueryClientProvider,
        apiGet,
        notifyMaintenanceState,
        MaintenanceGate,
        BackupSection,
        MailImportExportSection,
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

function setInputValue(input, value) {
  const setter = Object.getOwnPropertyDescriptor(win.HTMLInputElement.prototype, 'value').set
  setter.call(input, value)
  input.dispatchEvent(new win.Event('input', { bubbles: true }))
  input.dispatchEvent(new win.Event('change', { bubbles: true }))
}

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

test('backup creation blocks submit when passphrase < 12 chars or mismatch, and restore requires RESTORE', async (t) => {
  const prevFetch = globalThis.fetch
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.includes('/api/admin/backups')) {
      return jsonResponse({
        errorNo: 0,
        errorMsg: '',
        data: {
          items: [
            {
              name: 'pmail-v1.0.0-20261124T080000Z.pmailbak',
              size: 1024 * 1024,
              created_at: '2026-11-24T08:00:00Z',
              pmail_version: 'v1.0.0',
            },
          ],
          active_job: null,
        },
      })
    }
    return jsonResponse({ errorNo: 0, errorMsg: '', data: null })
  }
  t.after(() => { globalThis.fetch = prevFetch })

  const host = win.document.createElement('div')
  win.document.body.append(host)
  const qc = new mod.QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
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
        React.createElement(mod.BackupSection),
      ),
    )
  })
  await mod.act(async () => { await new Promise((r) => setTimeout(r, 20)) })

  const passInput = host.querySelector('#backup_passphrase')
  const confirmInput = host.querySelector('#backup_confirm_passphrase')
  const startBtn = [...host.querySelectorAll('button')].find((b) =>
    b.textContent?.includes('开始备份'),
  )
  assert.ok(passInput && confirmInput && startBtn)
  assert.equal(startBtn.disabled, true)

  // Short passphrase (< 12 chars) -> still disabled + validation error shown
  await mod.act(async () => {
    setInputValue(passInput, 'short123')
    setInputValue(confirmInput, 'short123')
  })
  assert.equal(startBtn.disabled, true)
  assert.match(host.textContent || '', /至少需要 12 个字符/)

  // >= 12 chars but mismatch -> still disabled + mismatch error shown
  await mod.act(async () => {
    setInputValue(passInput, 'very-strong-passphrase-123')
    setInputValue(confirmInput, 'different-passphrase-456')
  })
  assert.equal(startBtn.disabled, true)
  assert.match(host.textContent || '', /两次输入的口令不一致/)

  // Matching >= 12 chars -> enabled
  await mod.act(async () => {
    setInputValue(confirmInput, 'very-strong-passphrase-123')
  })
  assert.equal(startBtn.disabled, false)

  // Open restore dialog and verify RESTORE confirmation gate
  const restoreBtn = [...host.querySelectorAll('button')].find((b) =>
    b.textContent?.trim() === '恢复',
  )
  assert.ok(restoreBtn, 'missing restore button')
  await mod.act(async () => restoreBtn.click())

  const restorePass = win.document.querySelector('#restore_passphrase')
  const restoreConfirm = win.document.querySelector('#restore_confirm_input')
  const confirmRestoreBtn = [...win.document.querySelectorAll('button')].find((b) =>
    b.textContent?.includes('确认恢复'),
  )
  assert.ok(restorePass && restoreConfirm && confirmRestoreBtn)
  assert.equal(confirmRestoreBtn.disabled, true)

  // Passphrase entered, but RESTORE not typed -> still disabled
  await mod.act(async () => {
    setInputValue(restorePass, 'very-strong-passphrase-123')
    setInputValue(restoreConfirm, 'restore')
  })
  assert.equal(confirmRestoreBtn.disabled, true)

  // Exact RESTORE typed -> enabled
  await mod.act(async () => {
    setInputValue(restoreConfirm, 'RESTORE')
  })
  assert.equal(confirmRestoreBtn.disabled, false)
})

test('HTTP 503 maintenance triggers MaintenanceGate to display unified maintenance screen', async (t) => {
  const prevFetch = globalThis.fetch
  globalThis.fetch = async () =>
    jsonResponse({ errorNo: 503, errorMsg: 'maintenance', data: { code: 'maintenance' } }, 503)
  t.after(() => { globalThis.fetch = prevFetch })

  const host = win.document.createElement('div')
  win.document.body.append(host)
  const rootNode = mod.createRoot(host)
  t.after(async () => {
    await mod.act(async () => rootNode.unmount())
    host.remove()
  })

  await mod.act(async () => {
    rootNode.render(
      React.createElement(
        mod.MaintenanceGate,
        null,
        React.createElement('div', { id: 'normal-app' }, 'Normal App'),
      ),
    )
  })
  assert.ok(host.querySelector('#normal-app'))

  await mod.act(async () => {
    await assert.rejects(() => mod.apiGet('/api/email/list'))
  })

  assert.equal(host.querySelector('#normal-app'), null)
  assert.match(host.textContent || '', /系统维护中/)
})

test('maintenance on a cold raw-JSON bootstrap is shown once the gate mounts', async (t) => {
  mod.notifyMaintenanceState(false)
  const prevFetch = globalThis.fetch
  globalThis.fetch = async () =>
    jsonResponse({ errorNo: 503, errorMsg: 'maintenance', data: { code: 'maintenance' } }, 503)
  t.after(() => {
    globalThis.fetch = prevFetch
    mod.notifyMaintenanceState(false)
  })

  // /api/bootstrap uses the raw JSON path and fails before the gate exists.
  await assert.rejects(() => mod.apiGet('/api/bootstrap', { rawJson: true }), (err) => err.status === 503)

  const host = win.document.createElement('div')
  win.document.body.append(host)
  const rootNode = mod.createRoot(host)
  t.after(async () => {
    await mod.act(async () => rootNode.unmount())
    host.remove()
  })
  await mod.act(async () => {
    rootNode.render(
      React.createElement(
        mod.MaintenanceGate,
        null,
        React.createElement('div', { id: 'normal-app' }, 'Normal App'),
      ),
    )
  })
  assert.equal(host.querySelector('#normal-app'), null)
  assert.match(host.textContent || '', /系统维护中/)
})

test('MailImportExportSection displays accurate total, imported, skipped, and failed counts', async (t) => {
  const prevFetch = globalThis.fetch
  const prevXHR = globalThis.XMLHttpRequest
  globalThis.XMLHttpRequest = undefined
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.includes('/api/group/list')) {
      return jsonResponse({ errorNo: 0, errorMsg: '', data: [] })
    }
    if (u.includes('/api/email/import')) {
      return jsonResponse({
        errorNo: 0,
        errorMsg: '',
        data: {
          id: 'imp-1',
          state: 'done',
          total: 42,
          imported: 36,
          skipped: 5,
          failed: 1,
        },
      })
    }
    return jsonResponse({ errorNo: 0, errorMsg: '', data: null })
  }
  t.after(() => {
    globalThis.fetch = prevFetch
    globalThis.XMLHttpRequest = prevXHR
  })

  const host = win.document.createElement('div')
  win.document.body.append(host)
  const qc = new mod.QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
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
        React.createElement(mod.MailImportExportSection),
      ),
    )
  })

  const fileInput = host.querySelector('input[type="file"][accept=".eml,.mbox,.zip"]')
  assert.ok(fileInput, 'missing import file input')
  const fakeFile = new win.File(['From: a@b.com\n\nHi'], 'archive.mbox', { type: 'text/plain' })
  Object.defineProperty(fileInput, 'files', { value: [fakeFile], configurable: true })

  await mod.act(async () => {
    fileInput.dispatchEvent(new win.Event('change', { bubbles: true }))
    await new Promise((r) => setTimeout(r, 30))
  })

  assert.equal(host.querySelector('[data-import-stat="total"]')?.textContent, '42')
  assert.equal(host.querySelector('[data-import-stat="imported"]')?.textContent, '36')
  assert.equal(host.querySelector('[data-import-stat="skipped"]')?.textContent, '5')
  assert.equal(host.querySelector('[data-import-stat="failed"]')?.textContent, '1')
})
