import assert from 'node:assert/strict'
import path from 'node:path'
import test from 'node:test'
import { build } from 'esbuild'

const root = path.resolve(import.meta.dirname, '..')
const result = await build({
  stdin: {
    contents: `
      export { QueryClient, QueryObserver } from '@tanstack/react-query'
      export { authApi } from './src/api/auth'
      export { setNeedBootstrapHandler, setUnauthorizedHandler } from './src/api/client'
      export { createLogoutHandler } from './src/features/auth/logout'
      export { queryKeys } from './src/hooks/useMailQueries'
    `,
    resolveDir: root,
  },
  alias: { '@': path.join(root, 'src') },
  bundle: true,
  format: 'esm',
  platform: 'browser',
  write: false,
})
const bundle = Buffer.from(result.outputFiles[0].text).toString('base64')
const { QueryClient, QueryObserver, authApi, createLogoutHandler, queryKeys, setNeedBootstrapHandler, setUnauthorizedHandler } =
  await import(`data:text/javascript;base64,${bundle}`)

const signedIn = { logged_in: true, need_register: false, domains: ['example.test'] }
const signedOut = { ...signedIn, logged_in: false, domains: [] }
const inboxKey = ['emails', 'inbox']
const signedOutResponse = () => Response.json(signedOut)
const accepted = () => Response.json({ errorNo: 0, errorMsg: '', data: 'success' })
const forbidden = (errorMsg) => Response.json({ errorNo: 405, errorMsg }, { status: 403 })

function fixture(t, logoutResponse, bootstrapResponse = signedOutResponse) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } })
  queryClient.setQueryData(inboxKey, ['cached mail'])
  queryClient.setQueryData(queryKeys.bootstrap, signedIn)
  const bootstrapOptions = queryClient.defaultQueryOptions({
    queryKey: queryKeys.bootstrap,
    queryFn: ({ signal }) => authApi.bootstrap(signal),
    staleTime: 60_000,
  })
  const gateObserver = new QueryObserver(queryClient, bootstrapOptions)
  const unsubscribe = gateObserver.subscribe(() => {})
  const events = []
  const requests = []
  const handler = createLogoutHandler(queryClient)
  const actions = {
    setPending: (pending) => events.push(pending ? 'pending' : 'idle'),
    onError: (key) => events.push(key),
    onSuccess: () => events.push('success toast'),
    navigateToLogin: () => {
      // Match the observer snapshot that RouteGate reads again on navigation.
      events.push(['navigate /login', gateObserver.getOptimisticResult(bootstrapOptions).data])
    },
  }
  t.mock.method(globalThis, 'fetch', (url, options) => {
    requests.push([url, options.method, options.cache])
    if (url === '/api/logout') return logoutResponse()
    if (url === '/api/bootstrap') return bootstrapResponse()
    throw new Error(`Unexpected request: ${url}`)
  })
  setUnauthorizedHandler(() => events.push('global auth redirect'))
  setNeedBootstrapHandler(() => events.push('global bootstrap redirect'))
  t.after(() => {
    setUnauthorizedHandler(null)
    setNeedBootstrapHandler(null)
    unsubscribe()
    queryClient.clear()
  })
  return { queryClient, events, requests, logout: () => handler(actions) }
}

for (const [name, response] of [
  ['successful logout', accepted],
  ['401 with expired session', () => Response.json({ errorNo: 403, errorMsg: '未登录' }, { status: 401 })],
  ['plain HTTP 401', () => new Response('Unauthorized', { status: 401 })],
  ['legacy NeedLogin response', () => Response.json({ errorNo: 403, errorMsg: '未登录' })],
]) {
  test(`${name} clears private cache and sets fresh bootstrap before navigation`, async (t) => {
    const { queryClient, events, requests, logout } = fixture(t, response)
    await logout()
    assert.deepEqual(events, ['pending', 'success toast', ['navigate /login', signedOut], 'idle'])
    assert.deepEqual(requests, [['/api/logout', 'POST', undefined], ['/api/bootstrap', 'GET', 'no-store']])
    assert.equal(queryClient.getQueryData(inboxKey), undefined)
    assert.deepEqual(queryClient.getQueryData(queryKeys.bootstrap), signedOut)
  })
}

for (const [name, response, errorKey] of [
  ['403 origin_forbidden', () => forbidden('origin_forbidden'), 'auth.logoutOriginForbidden'],
  ['localized 403 from the real backend', () => forbidden('此请求必须来自同源页面。'), 'auth.logoutOriginForbidden'],
  ['bare HTTP 403 is not NeedLogin', () => new Response('Forbidden', { status: 403 }), 'auth.logoutOriginForbidden'],
  ['coded origin rejection on legacy HTTP 200', () => Response.json({ errorNo: 405, errorMsg: 'origin_forbidden' }), 'auth.logoutOriginForbidden'],
  ['network error', () => { throw new TypeError('Failed to fetch') }, 'auth.logoutFailed'],
  ['5xx', () => new Response('Bad Gateway', { status: 502 }), 'auth.logoutFailed'],
]) {
  test(`${name} reports failure without clearing cache or navigating`, async (t) => {
    const { queryClient, events, requests, logout } = fixture(t, response)
    await logout()
    assert.deepEqual(events, ['pending', errorKey, 'idle'])
    assert.deepEqual(requests, [['/api/logout', 'POST', undefined]])
    assert.deepEqual(queryClient.getQueryData(inboxKey), ['cached mail'])
    assert.deepEqual(queryClient.getQueryData(queryKeys.bootstrap), signedIn)
  })
}

test('one logout remains in flight until fresh bootstrap is ready', async (t) => {
  let resolveBootstrap
  let markBootstrapRequested
  const bootstrapRequested = new Promise((resolve) => { markBootstrapRequested = resolve })
  const bootstrap = new Promise((resolve) => { resolveBootstrap = resolve })
  const { events, logout, requests } = fixture(t, accepted, () => {
    markBootstrapRequested()
    return bootstrap
  })
  const first = logout()
  await logout()
  await bootstrapRequested
  await logout()
  assert.deepEqual(events, ['pending'])
  assert.equal(requests.filter(([url]) => url === '/api/logout').length, 1)
  resolveBootstrap(signedOutResponse())
  await first
  assert.deepEqual(events, ['pending', 'success toast', ['navigate /login', signedOut], 'idle'])
})

test('an in-flight signed-in bootstrap cannot overwrite the post-logout snapshot', async (t) => {
  const { events, queryClient, logout } = fixture(t, accepted)
  let resolveOldBootstrap
  const oldBootstrap = new Promise((resolve) => { resolveOldBootstrap = resolve })
  let oldSignal
  const oldQuery = queryClient.fetchQuery({
    queryKey: queryKeys.bootstrap,
    queryFn: ({ signal }) => {
      oldSignal = signal
      return oldBootstrap
    },
    staleTime: 0,
  }).catch(() => undefined)
  assert.equal(oldSignal?.aborted, false)
  await logout()
  await oldQuery
  assert.equal(oldSignal.aborted, true)
  resolveOldBootstrap(signedIn)
  await oldBootstrap
  assert.deepEqual(events, ['pending', 'success toast', ['navigate /login', signedOut], 'idle'])
  assert.deepEqual(queryClient.getQueryData(queryKeys.bootstrap), signedOut)
})

test('a failed logout can be retried', async (t) => {
  let attempt = 0
  const { events, logout } = fixture(t, () => ++attempt === 1 ? forbidden('origin_forbidden') : accepted())
  await logout()
  await logout()
  assert.deepEqual(events, ['pending', 'auth.logoutOriginForbidden', 'idle', 'pending', 'success toast', ['navigate /login', signedOut], 'idle'])
})

test('confirmed logout with unavailable bootstrap leaves no stale signed-in snapshot for the gate', async (t) => {
  const { events, queryClient, logout } = fixture(t, accepted, () => new Response(null, { status: 503 }))
  await logout()
  assert.deepEqual(events, ['pending', 'success toast', ['navigate /login', undefined], 'idle'])
  assert.equal(queryClient.getQueryData(inboxKey), undefined)
  assert.equal(queryClient.getQueryData(queryKeys.bootstrap), undefined)
})

test('a fresh bootstrap that is still signed in does not claim success', async (t) => {
  const { events, queryClient, logout } = fixture(t, accepted, () => Response.json(signedIn))
  await logout()
  assert.deepEqual(events, ['pending', 'auth.logoutFailed', 'idle'])
  assert.deepEqual(queryClient.getQueryData(inboxKey), ['cached mail'])
})
