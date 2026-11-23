import assert from 'node:assert/strict'
import path from 'node:path'
import test from 'node:test'
import { build } from 'esbuild'

const root = path.resolve(import.meta.dirname, '..')
const bundle = await build({
  stdin: {
    resolveDir: root, loader: 'tsx', contents: `
      import { renderToStaticMarkup } from 'react-dom/server'
      import { AddressPicker } from './src/features/mail/AddressPicker'
      import './src/i18n'
      export * from './src/features/mail/addressViews'
      export function picker(counts, filter) {
        return renderToStaticMarkup(<AddressPicker counts={counts} selection={filter} onChange={() => {}} />)
      }
    `,
  },
  alias: { '@': path.join(root, 'src') }, jsx: 'automatic', bundle: true, format: 'esm', platform: 'node', write: false,
  banner: { js: `import { createRequire } from 'node:module'; const require = createRequire(${JSON.stringify(import.meta.url)});` },
})
const view = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString('base64')}`)
const primary = { id: 1, address: 'main@example.test', can_send: true, is_primary: true, unread: 5 }
const alias = { id: 2, address: 'alias@example.test', can_send: true, is_primary: false, unread: 3 }
const readOnly = { id: 3, address: 'inbound@example.test', can_send: false, is_primary: false, unread: 10 }
const counts = { addresses: [primary, alias, readOnly], all_unread: 12, other: { has_mail: true, unread: 2 } }

test('address picker hides for one address without catch-all and shows selectable counts otherwise', () => {
  assert.equal(view.picker({ ...counts, addresses: [primary], other: { has_mail: false, unread: 0 } }, 'all'), '')
  const html = view.picker(counts, 2)
  assert.match(html, /value="2" selected="">alias@example.test（3 未读）/)
  assert.match(html, /value="other">其他地址（2 未读）/)
  assert.doesNotMatch(view.picker({ ...counts, other: { has_mail: false, unread: 0 } }, 'all'), /value="other"/)
})

test('folder navigation preserves selection and deleted aliases fall back to all', () => {
  assert.equal(view.readAddressFilter('2'), 2)
  assert.equal(view.readAddressFilter('9999999999999999999999'), 'all')
  assert.equal(view.availableAddressFilter(2, counts), 2)
  assert.equal(view.availableAddressFilter(2, { ...counts, addresses: [primary] }), 'all')
  assert.equal(view.availableAddressFilter(1, { ...counts, addresses: [primary], other: { has_mail: false, unread: 0 } }), 'all')
  assert.equal(view.availableAddressFilter('other', { ...counts, other: { has_mail: false, unread: 0 } }), 'all')
  assert.equal(view.folderURL('sent', 2), '/mail/sent?address=2')
  assert.equal(view.folderURL('group-8', 'other'), '/mail/group-8?address=other')
})

test('new mail uses selected sendable alias, while replies use matched addresses only', () => {
  assert.equal(view.composeFrom(counts.addresses, 2), alias.address)
  assert.equal(view.composeFrom(counts.addresses, 'other'), primary.address)
  assert.equal(view.composeFrom(counts.addresses, 3), primary.address)
  assert.equal(view.composeFrom(counts.addresses, 2, [primary, alias]), alias.address)
  assert.equal(view.composeFrom(counts.addresses, 'all', [alias]), alias.address)
  assert.equal(view.composeFrom(counts.addresses, 2, [readOnly]), primary.address)
})

test('local drafts also respect address and keyword filtering', () => {
  const drafts = [
    { id: 'a', fromAddress: alias.address, subject: 'Quarterly check-in', text: 'memo' },
    { id: 'b', fromAddress: primary.address, subject: 'Other memo', text: '' },
  ]
  assert.deepEqual(view.draftsInAddressView(drafts, 2, counts, 'quarterly').map(({ id }) => id), ['a'])
  assert.deepEqual(view.draftsInAddressView(drafts, 'other', counts, ''), [])
  assert.equal(view.draftsInAddressView(drafts, 'all', counts, 'memo').length, 2)
})
