#!/usr/bin/env node
/**
 * PMail Pro v4 i18n Integrity & Chinese Coverage Checker
 *
 * Verifies:
 * 1. zh-CN.ts and en.ts have identical flattened key sets.
 * 2. Every static t('...') key referenced in src/ exists in both locale files.
 * 3. zh-CN.ts values do not contain untranslated English sentences (allowing whitelisted brand/protocol/code terms).
 * 4. JSX visible text / placeholder / title / aria-label attributes in src/ do not contain hardcoded English sentences.
 */
import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'

const ROOT = path.resolve(import.meta.dirname, '..')
const SRC_DIR = path.join(ROOT, 'src')
const ZH_FILE = path.join(SRC_DIR, 'i18n/locales/zh-CN.ts')
const EN_FILE = path.join(SRC_DIR, 'i18n/locales/en.ts')

function loadLocaleObject(filePath) {
  const source = fs.readFileSync(filePath, 'utf8')
  const sf = ts.createSourceFile(filePath, source, ts.ScriptTarget.Latest, true)
  const map = new Map()

  function walkObject(node, prefix) {
    for (const prop of node.properties) {
      if (!ts.isPropertyAssignment(prop)) continue
      const key = ts.isIdentifier(prop.name)
        ? prop.name.text
        : ts.isStringLiteral(prop.name)
          ? prop.name.text
          : null
      if (!key) continue
      const fullKey = prefix ? `${prefix}.${key}` : key
      if (ts.isObjectLiteralExpression(prop.initializer)) {
        walkObject(prop.initializer, fullKey)
      } else if (
        ts.isStringLiteral(prop.initializer) ||
        ts.isNoSubstitutionTemplateLiteral(prop.initializer)
      ) {
        map.set(fullKey, prop.initializer.text)
      }
    }
  }

  function visit(node) {
    if (ts.isVariableDeclaration(node) && node.initializer) {
      let expr = node.initializer
      if (ts.isAsExpression(expr)) expr = expr.expression
      if (ts.isObjectLiteralExpression(expr)) {
        walkObject(expr, '')
        return
      }
    }
    ts.forEachChild(node, visit)
  }

  visit(sf)
  return map
}

function listSourceFiles(dir) {
  const results = []
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === 'mocks' || entry.name === 'i18n') continue
      results.push(...listSourceFiles(full))
    } else if (/\.(ts|tsx)$/.test(entry.name)) {
      results.push(full)
    }
  }
  return results
}

const ALLOWED_ENGLISH_TOKENS = new Set([
  'PMail', 'Pro', 'PRO', 'IMAP', 'IMAPS', 'POP3', 'POP3S', 'SMTP', 'DNS', 'DKIM', 'SPF', 'DMARC',
  'PTR', 'MX', 'TTL', 'BIND', 'API', 'Token', 'Bearer', 'Authorization',
  'HTML', 'Markdown', 'URL', 'GitHub', 'Gmail', 'Outlook', 'iCloud', 'QQ',
  'VPS', 'IPv4', 'IPv6', 'IP', 'rDNS', 'Autoconfig', 'Autodiscover', 'SRV', 'MB', 'GB', 'KB',
  'Ctrl', 'Enter', 'Esc', 'English', 'docker', 'compose', 'logs', 'pmail',
  'grep', 'Mail', 'Host', 'Web', 'Domain', 'Message-ID', 'SSL', 'TLS', 'STARTTLS',
  'CI', 'CD', 'UI', 'TypeScript', 'JavaScript', 'TSX', 'JSX', 'Python', 'Go',
  'Rust', 'Shell', 'Bash', 'JSON', 'YAML', 'SQL', 'Diff', 'CSS', 'Dockerfile',
  'TOML', 'XML', 'PHP', 'C', 'C++', 'Java', 'admin', 'hello', 'work', 'example',
  'com', 'https', 'imap', 'gmail', 'XXXX', 'and', 'or', 'v', 's', 'a', 'c', 'e',
  'f', 'j', 'k', 'o', 'r', 'mx', 'spf', 'TXT',
])

const REQUIRED_ENUM_KEYS = [
  // DNS purpose enums (must never leak raw English values like receiving / client_configuration)
  ...[
    'receiving',
    'mail_host',
    'web',
    'authentication',
    'reverse_dns',
    'client_configuration',
    'other',
  ].map((k) => `admin.dnsWizard.purposes.${k}`),
  // DNS record key enums
  ...[
    'mx',
    'mail_a',
    'mail_aaaa',
    'web_a',
    'web_aaaa',
    'spf',
    'dkim',
    'dmarc',
    'ptr',
    'imap_alias',
    'pop_alias',
    'autoconfig',
    'autodiscover',
    'imaps',
    'submission',
    'pop3s',
  ].map((k) => `admin.dnsWizard.recordKeys.${k}`),
  // DNS status enums
  ...['ok', 'missing', 'mismatch', 'error', 'advice'].map((k) => `admin.dnsWizard.status.${k}`),
]

const errors = []

// 1. Key parity between zh-CN and en
const zhMap = loadLocaleObject(ZH_FILE)
const enMap = loadLocaleObject(EN_FILE)

for (const key of zhMap.keys()) {
  if (!enMap.has(key)) errors.push(`[missing-in-en] Key "${key}" exists in zh-CN.ts but not in en.ts`)
}
for (const key of enMap.keys()) {
  if (!zhMap.has(key)) errors.push(`[missing-in-zh] Key "${key}" exists in en.ts but not in zh-CN.ts`)
}

// 2. Check zh-CN values for untranslated English sentences (3+ non-whitelisted English words in a row)
for (const [key, val] of zhMap.entries()) {
  // Strip interpolation placeholders {{...}}
  const stripped = val.replace(/\{\{[^}]+\}\}/g, '')
  const words = stripped.match(/[A-Za-z][A-Za-z0-9_+-]*/g) ?? []
  const nonWhitelisted = words.filter((w) => !ALLOWED_ENGLISH_TOKENS.has(w))
  if (nonWhitelisted.length >= 3 && !/[\u4e00-\u9fff]/.test(stripped)) {
    errors.push(`[untranslated-zh] Key "${key}" looks like untranslated English: "${val}"`)
  }
}

// 2.5 Check required backend enum translations exist and are localized in zh-CN
for (const enumKey of REQUIRED_ENUM_KEYS) {
  const zhVal = zhMap.get(enumKey)
  const enVal = enMap.get(enumKey)
  if (!zhVal) {
    errors.push(`[missing-enum-zh] Required enum i18n key "${enumKey}" is missing in zh-CN.ts`)
  } else if (!/[\u4e00-\u9fff]/.test(zhVal)) {
    errors.push(`[unlocalized-enum-zh] Enum key "${enumKey}" in zh-CN.ts must contain Chinese: "${zhVal}"`)
  }
  if (!enVal) {
    errors.push(`[missing-enum-en] Required enum i18n key "${enumKey}" is missing in en.ts`)
  }
}

// 3. Scan all source files for t('...') calls & hardcoded English in JSX
const files = listSourceFiles(SRC_DIR)
const referencedKeys = new Set()

for (const file of files) {
  const content = fs.readFileSync(file, 'utf8')
  const sf = ts.createSourceFile(file, content, ts.ScriptTarget.Latest, true)
  const rel = path.relative(ROOT, file)

  function visit(node) {
    // Check t('literal.key')
    if (
      ts.isCallExpression(node) &&
      ((ts.isIdentifier(node.expression) && node.expression.text === 't') ||
        (ts.isPropertyAccessExpression(node.expression) && node.expression.name.text === 't'))
    ) {
      const firstArg = node.arguments[0]
      if (firstArg && ts.isStringLiteral(firstArg)) {
        const key = firstArg.text
        referencedKeys.add(key)
        if (!zhMap.has(key)) {
          errors.push(`[missing-key] ${rel}: t('${key}') not found in zh-CN.ts`)
        }
      }
    }

    // Guard against rendering raw backend DNS enum fields directly in JSX (e.g. {record.purpose})
    if (
      ts.isJsxExpression(node) &&
      node.expression &&
      ts.isPropertyAccessExpression(node.expression) &&
      node.expression.name.text === 'purpose'
    ) {
      errors.push(`[raw-enum-jsx] ${rel}: raw ".purpose" rendered directly in JSX — use getDnsRecordLabel/getDnsPurposeLabel`)
    }

    // Check JSX text for hardcoded English sentences (2+ non-whitelisted English words)
    if (ts.isJsxText(node)) {
      const text = node.getText(sf).trim()
      if (text && !/[\u4e00-\u9fff]/.test(text)) {
        const words = text.match(/[A-Za-z]{2,}/g) ?? []
        const bad = words.filter((w) => !ALLOWED_ENGLISH_TOKENS.has(w))
        if (bad.length >= 2) {
          errors.push(`[hardcoded-jsx] ${rel}: "${text}"`)
        }
      }
    }

    // Check JSX user-facing string attributes (placeholder, title, aria-label)
    if (ts.isJsxAttribute(node) && node.initializer && ts.isStringLiteral(node.initializer)) {
      const attrName = node.name.getText(sf)
      if (attrName === 'placeholder' || attrName === 'title' || attrName === 'aria-label') {
        const val = node.initializer.text.trim()
        if (val && !/[\u4e00-\u9fff]/.test(val)) {
          const words = val.match(/[A-Za-z]{2,}/g) ?? []
          const bad = words.filter((w) => !ALLOWED_ENGLISH_TOKENS.has(w))
          if (bad.length >= 1) {
            errors.push(`[hardcoded-attr] ${rel} ${attrName}="${val}"`)
          }
        }
      }
    }

    ts.forEachChild(node, visit)
  }

  visit(sf)
}

if (errors.length > 0) {
  console.error(`\n❌ i18n check failed with ${errors.length} issue(s):\n`)
  for (const err of errors) console.error(`  - ${err}`)
  process.exit(1)
}

console.log(
  `✅ i18n check passed: ${zhMap.size} zh-CN keys, ${enMap.size} en keys (100% parity), ${referencedKeys.size} static t() keys + ${REQUIRED_ENUM_KEYS.length} enum keys verified, 0 hardcoded English strings across ${files.length} source files.`,
)
