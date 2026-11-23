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
for (const name of ['document', 'HTMLElement', 'Node', 'HTMLInputElement']) globalThis[name] = win[name]
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
      import { MemoryRouter, Routes, Route } from 'react-router'
      import { ThemeProvider } from './src/lib/ThemeProvider'
      import { LoginPage } from './src/features/auth/LoginPage'
      import { AccountCredentials } from './src/features/settings/sections/AccountCredentials'
      export { act }
      export function mount(host, page) {
        const root = createRoot(host)
        const query = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
        root.render(<QueryClientProvider client={query}>
          <ThemeProvider>
            <MemoryRouter initialEntries={[page]}>
              <Routes>
                <Route path="/login" element={<LoginPage />} />
                <Route path="/settings" element={<AccountCredentials userID={7} />} />
                <Route path="/mail/inbox" element={<div id="signed-in" />} />
              </Routes>
            </MemoryRouter>
          </ThemeProvider>
        </QueryClientProvider>)
        return () => { root.unmount(); query.clear() }
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
      // Keep React's Node act() implementation (CJS module.require('timers'));
      // bundling React would select its browser fallback and leave MessagePorts open.
      plugin.onResolve({ filter: /^react$/ }, () => ({ path: 'react', namespace: 'native-react' }))
      plugin.onLoad({ filter: /.*/, namespace: 'native-react' }, () => ({
        contents: `const { createRequire } = require('node:module'); module.exports = createRequire(${JSON.stringify(import.meta.url)})('react');`,
        loader: 'js',
      }))
    },
  }],
  banner: { js: `import { createRequire } from 'node:module'; const require = createRequire(${JSON.stringify(import.meta.url)});` },
})
const { act, mount } = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString('base64')}`)
const ok = (data) => Response.json({ errorNo: 0, errorMsg: '', data })

async function fixture(t, page, handlers) {
  const calls = []
  const originalFetch = globalThis.fetch
  const host = win.document.createElement('div')
  win.document.body.append(host)
  globalThis.fetch = async (url, init) => {
    const key = `${init.method} ${url}`
    const body = init.body ? JSON.parse(init.body) : undefined
    calls.push([key, body])
    assert.ok(handlers[key], `unexpected API call: ${key}`)
    return handlers[key](body)
  }
  let cleanup
  t.after(async () => {
    if (cleanup) await act(async () => cleanup())
    host.remove()
    globalThis.fetch = originalFetch
  })
  await act(async () => { cleanup = mount(host, page) })
  return { host, calls }
}

async function waitFor(assertion) {
  let lastError
  for (let i = 0; i < 30; i++) {
    try { return assertion() } catch (err) { lastError = err }
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)) })
  }
  throw lastError
}

function button(host, text) {
  const target = [...host.querySelectorAll('button')].find((b) => b.textContent.trim() === text)
  assert.ok(target, `missing button: ${text}`)
  return target
}

function input(element, text) {
  assert.ok(element, 'missing input')
  Object.getOwnPropertyDescriptor(win.HTMLInputElement.prototype, 'value').set.call(element, text)
  element.dispatchEvent(new win.Event('input', { bubbles: true }))
}

async function submit(form) {
  assert.ok(form)
  await act(async () => { form.dispatchEvent(new win.Event('submit', { bubbles: true, cancelable: true })) })
}

test('password challenge renders the second step and uses a recovery code without re-sending password', async (t) => {
  const { host, calls } = await fixture(t, '/login', {
    'POST /api/login': () => ok({ two_factor_required: true, challenge: 'ephemeral-fixture', expires_in: 300 }),
    'POST /api/login/2fa': () => ok({ account: 'test', name: 'Test', is_admin: 0, domains: [] }),
  })
  await act(async () => {
    input(host.querySelector('#login_account'), 'test')
    input(host.querySelector('#login_password'), 'login-only-password')
  })
  await submit(host.querySelector('form'))
  assert.deepEqual(calls, [['POST /api/login', { account: 'test', password: 'login-only-password' }]])
  assert.ok(host.querySelector('input[autocomplete="one-time-code"]'))
  assert.equal(host.querySelector('#login_password'), null)
  assert.equal(host.querySelector('#signed-in'), null)
  await act(async () => button(host, '使用恢复码').click())
  const recoveryInput = host.querySelector('form input[autocomplete="off"]')
  await act(async () => input(recoveryInput, 'ABCD-EFGH-JKLM-NPQR'))
  await submit(host.querySelector('form'))
  assert.deepEqual(calls[1], ['POST /api/login/2fa', { challenge: 'ephemeral-fixture', recovery: true, code: 'ABCD-EFGH-JKLM-NPQR' }])
  assert.ok(host.querySelector('#signed-in'))
})

test('settings renders a local QR and manual key, then shows recovery codes only after enable', async (t) => {
  const fixtureKey = 'JBSWY3DPEHPK3PXP'
  const { host, calls } = await fixture(t, '/settings', {
    'GET /api/2fa': () => ok({ enabled: false }),
    'GET /api/app_passwords': () => ok([]),
    'POST /api/2fa/setup': () => ok({ secret: fixtureKey, otpauth_uri: `otpauth://totp/fixture?secret=${fixtureKey}` }),
    'POST /api/2fa/enable': () => ok({ enabled: true, recovery_codes: ['ABCD-EFGH-JKLM-NPQR'] }),
  })
  const setup = await waitFor(() => button(host, '设置两步验证'))
  await act(async () => setup.click())
  assert.equal(host.querySelector('#totp-manual-key')?.value, fixtureKey)
  assert.match(host.querySelector('section[aria-labelledby="factor-title"] img')?.getAttribute('src') ?? '', /^data:image\/svg\+xml,/)
  assert.doesNotMatch(host.textContent, /ABCD-EFGH/)
  const codeInput = host.querySelector('input[autocomplete="one-time-code"]')
  await act(async () => input(codeInput, '123456'))
  await submit(codeInput.form)
  assert.deepEqual(calls.find(([key]) => key.endsWith('/enable')), ['POST /api/2fa/enable', { code: '123456' }])
  assert.equal(host.querySelector('#totp-manual-key'), null)
  assert.match(host.textContent, /两步验证已开启/)
  assert.match(host.textContent, /ABCD-EFGH-JKLM-NPQR/)
  await act(async () => button(host, '已保存').click())
  assert.doesNotMatch(host.textContent, /ABCD-EFGH/)
})
