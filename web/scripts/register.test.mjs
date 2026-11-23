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
const { renderRegistration } = await import(`data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString('base64')}`)
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
