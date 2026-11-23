import assert from 'node:assert/strict'
import path from 'node:path'
import test from 'node:test'
import { build } from 'esbuild'

const root = path.resolve(import.meta.dirname, '..')
const result = await build({
  stdin: {
    contents: `
      import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
      import { renderToStaticMarkup } from 'react-dom/server'
      import { MemoryRouter } from 'react-router'
      import { RegisterPage } from './src/features/auth/RegisterPage'
      import { queryKeys } from './src/hooks/useMailQueries'
      import { ThemeProvider } from './src/lib/ThemeProvider'
      export { guessMailDomain, DOMAIN_PATTERN, waitForDomainSetup } from './src/features/auth/mailDomain'

      export function renderRegistration(bootstrap) {
        const client = new QueryClient()
        client.setQueryData(queryKeys.bootstrap, bootstrap)
        return renderToStaticMarkup(
          <QueryClientProvider client={client}>
            <ThemeProvider>
              <MemoryRouter initialEntries={['/register']}>
                <RegisterPage />
              </MemoryRouter>
            </ThemeProvider>
          </QueryClientProvider>,
        )
      }
    `,
    resolveDir: root,
    loader: 'tsx',
  },
  alias: { '@': path.join(root, 'src') },
  jsx: 'automatic',
  bundle: true,
  format: 'esm',
  platform: 'node',
  banner: { js: `import { createRequire } from 'node:module'; const require = createRequire(${JSON.stringify(import.meta.url)});` },
  write: false,
})
const { renderRegistration, guessMailDomain, DOMAIN_PATTERN, waitForDomainSetup } = await import(`data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString('base64')}`)
const anonymous = {
  version: 'dev',
  need_register: true,
  logged_in: false,
  default_lang: 'zh-CN',
  web_domain: 'mail.xshi.cc',
  domains: [],
  features: { register_key: true },
}

test('registration renders the configured mail domain, not the web hostname', () => {
  const html = renderRegistration({ ...anonymous, primary_domain: 'xshi.cc' })
  assert.match(html, /@xshi\.cc<\/p>/)
  assert.doesNotMatch(html, /@mail\.xshi\.cc/)
})

test('registration does not guess a suffix from an older bootstrap response', () => {
  const html = renderRegistration(anonymous)
  assert.doesNotMatch(html, /@mail\.xshi\.cc/)
})

test('first-run page asks for the mail domain when none is configured', () => {
  const html = renderRegistration({ ...anonymous, need_domain: true, web_domain: '', primary_domain: '' })
  assert.match(html, /id="mail_domain"/)
  assert.doesNotMatch(renderRegistration({ ...anonymous, primary_domain: 'xshi.cc' }), /id="mail_domain"/)
})

test('mail domain is guessed from the address bar', () => {
  assert.equal(guessMailDomain('mail.xshi.cc'), 'xshi.cc')
  assert.equal(guessMailDomain('webmail.example.co.uk'), 'example.co.uk')
  assert.equal(guessMailDomain('xshi.cc'), 'xshi.cc')
  assert.equal(guessMailDomain('185.99.135.205'), '')
  assert.equal(guessMailDomain('localhost'), '')
  assert.ok(DOMAIN_PATTERN.test('7-89.cn'))
  assert.ok(DOMAIN_PATTERN.test('xn--fiqs8s.xn--fiqs8s'))
  assert.ok(!DOMAIN_PATTERN.test('mail'))
  assert.ok(!DOMAIN_PATTERN.test('http://x.cc'))
  assert.ok(!DOMAIN_PATTERN.test('x.a-'))
})


test('first-run polling requires a normal bootstrap and tolerates the listener handoff', async () => {
  const ready = { ...anonymous, need_register: false, primary_domain: 'example.test' }
  let calls = 0
  const result = await waitForDomainSetup(async () => {
    switch (++calls) {
      case 1: throw new Error('listener unavailable')
      case 2: return '<html>Proxy welcome page</html>'
      case 3: return null
      case 4: return { ...anonymous, need_domain: true }
      default: return ready
    }
  }, { intervalMs: 0 })
  assert.equal(result, ready)
  assert.equal(calls, 5)
})

test('first-run polling aborts an unresponsive bootstrap and times out', async () => {
  let aborts = 0
  const result = await waitForDomainSetup((signal) => new Promise((resolve, reject) => {
    signal.addEventListener('abort', () => { aborts++; reject(signal.reason) }, { once: true })
  }), { timeoutMs: 150, intervalMs: 500, requestMs: 5 })
  assert.equal(result, undefined)
  assert.equal(aborts, 1)
})
