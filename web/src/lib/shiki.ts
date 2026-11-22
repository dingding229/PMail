import type { HighlighterCore, LanguageRegistration, ThemedToken } from 'shiki/core'
import type { ResolvedTheme } from './theme'
import type { SupportedCodeLang } from './code-languages'

export type { SupportedCodeLang } from './code-languages'

const LANG_ALIASES: Record<string, SupportedCodeLang> = {
  text: 'text',
  plaintext: 'text',
  txt: 'text',
  go: 'go',
  golang: 'go',
  ts: 'typescript',
  typescript: 'typescript',
  tsx: 'tsx',
  js: 'javascript',
  javascript: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  jsx: 'jsx',
  py: 'python',
  python: 'python',
  rs: 'rust',
  rust: 'rust',
  sh: 'bash',
  shell: 'bash',
  bash: 'bash',
  zsh: 'bash',
  json: 'json',
  jsonc: 'json',
  yml: 'yaml',
  yaml: 'yaml',
  sql: 'sql',
  diff: 'diff',
  patch: 'diff',
  md: 'markdown',
  markdown: 'markdown',
  html: 'html',
  htm: 'html',
  css: 'css',
  scss: 'css',
  dockerfile: 'dockerfile',
  docker: 'dockerfile',
  toml: 'toml',
  xml: 'xml',
  svg: 'xml',
  c: 'c',
  h: 'c',
  cpp: 'cpp',
  cc: 'cpp',
  cxx: 'cpp',
  hpp: 'cpp',
  java: 'java',
  php: 'php',
}

type LangLoader = () => Promise<{ default: LanguageRegistration[] }>

const LANG_LOADERS: Partial<Record<SupportedCodeLang, LangLoader>> = {
  go: () => import('@shikijs/langs/go'),
  typescript: () => import('@shikijs/langs/typescript'),
  tsx: () => import('@shikijs/langs/tsx'),
  javascript: () => import('@shikijs/langs/javascript'),
  jsx: () => import('@shikijs/langs/jsx'),
  python: () => import('@shikijs/langs/python'),
  rust: () => import('@shikijs/langs/rust'),
  bash: () => import('@shikijs/langs/bash'),
  json: () => import('@shikijs/langs/json'),
  yaml: () => import('@shikijs/langs/yaml'),
  sql: () => import('@shikijs/langs/sql'),
  diff: () => import('@shikijs/langs/diff'),
  markdown: () => import('@shikijs/langs/markdown'),
  html: () => import('@shikijs/langs/html'),
  css: () => import('@shikijs/langs/css'),
  dockerfile: () => import('@shikijs/langs/dockerfile'),
  toml: () => import('@shikijs/langs/toml'),
  xml: () => import('@shikijs/langs/xml'),
  c: () => import('@shikijs/langs/c'),
  cpp: () => import('@shikijs/langs/c'),
  java: () => import('@shikijs/langs/java'),
  php: () => import('@shikijs/langs/php'),
}

export function normalizeCodeLanguage(raw?: string | null): SupportedCodeLang | null {
  if (!raw) return null
  const cleaned = raw
    .trim()
    .toLowerCase()
    .replace(/^(language|lang)-/, '')
  return LANG_ALIASES[cleaned] ?? null
}

/**
 * Lightweight heuristic language detector over a curated set of common languages
 * when `<pre><code>` carries no `language-*` / `lang-*` class.
 */
export function detectCodeLanguage(code: string): SupportedCodeLang {
  const sample = code.trim()
  if (!sample) return 'text'

  // Unified diff / git patch
  if (/^diff --git\s|^@@\s-\d+/m.test(sample) || (/^---\s/m.test(sample) && /^\+\+\+\s/m.test(sample))) {
    return 'diff'
  }
  // JSON
  if (
    (sample.startsWith('{') && sample.endsWith('}')) ||
    (sample.startsWith('[') && sample.endsWith(']'))
  ) {
    try {
      JSON.parse(sample)
      return 'json'
    } catch {
      // Not strict JSON
    }
  }
  // Go
  if (/^package\s+\w+/m.test(sample) || /\bfunc\s+(?:\([^)]+\)\s*)?\w+\s*\(/.test(sample) || /\bfmt\.Print/.test(sample)) {
    return 'go'
  }
  // Python
  if (/^(?:def|class|from\s+\S+\s+import|import\s+\w+)/m.test(sample) && /:\s*$/m.test(sample)) {
    return 'python'
  }
  // Rust
  if (/\bfn\s+\w+\s*\([^)]*\)\s*(?:->\s*[^ {]+)?\s*\{/.test(sample) && /\b(?:let\s+mut|impl|pub\s+fn|println!)/.test(sample)) {
    return 'rust'
  }
  // TypeScript / JavaScript
  if (/\b(?:interface|type)\s+[A-Z]\w*\s*(?:=|\{)/.test(sample) || /:\s*(?:string|number|boolean|Record<|Promise<)/.test(sample)) {
    return 'typescript'
  }
  if (/\b(?:const|let|var)\s+\w+\s*=|\bexport\s+(?:default\s+)?(?:function|const|class)\b/.test(sample)) {
    return 'javascript'
  }
  // SQL
  if (/^\s*(?:SELECT|INSERT\s+INTO|UPDATE|DELETE\s+FROM|CREATE\s+TABLE|ALTER\s+TABLE)\b/im.test(sample)) {
    return 'sql'
  }
  // Shell / Bash
  if (/^#!\/bin\/(?:ba|z)?sh/m.test(sample) || /^\s*(?:docker|kubectl|curl|grep|export|chmod|npm|go\s+test|git)\s+/m.test(sample)) {
    return 'bash'
  }
  // HTML / XML
  if (/^<!doctype\s+html|^<(?:html|div|section|table|p|span|head|body)\b/i.test(sample)) {
    return 'html'
  }
  // YAML
  if (/^[a-zA-Z0-9_-]+:\s*(?:\S|$)/m.test(sample) && /^\s+-\s+\S+/m.test(sample)) {
    return 'yaml'
  }
  // CSS
  if (/^[.#@]?[a-zA-Z0-9_-]+\s*\{[\s\S]*:\s*[^;]+;/m.test(sample)) {
    return 'css'
  }

  return 'text'
}

let highlighterPromise: Promise<HighlighterCore> | null = null
const loadedLangs = new Set<string>(['text'])

async function getHighlighter(): Promise<HighlighterCore> {
  if (!highlighterPromise) {
    highlighterPromise = (async () => {
      const [{ createHighlighterCore }, { createJavaScriptRegexEngine }, lightTheme, darkTheme] =
        await Promise.all([
          import('shiki/core'),
          import('shiki/engine/javascript'),
          import('@shikijs/themes/github-light'),
          import('@shikijs/themes/github-dark-default'),
        ])
      return createHighlighterCore({
        themes: [lightTheme.default, darkTheme.default],
        langs: [],
        engine: createJavaScriptRegexEngine(),
      })
    })()
  }
  return highlighterPromise
}

async function ensureLanguageLoaded(highlighter: HighlighterCore, lang: SupportedCodeLang): Promise<string> {
  const shikiLangId = lang === 'cpp' ? 'c' : lang
  if (shikiLangId === 'text' || loadedLangs.has(shikiLangId)) return shikiLangId
  const loader = LANG_LOADERS[lang]
  if (!loader) return 'text'
  try {
    const mod = await loader()
    await highlighter.loadLanguage(mod.default)
    loadedLangs.add(shikiLangId)
    return shikiLangId
  } catch {
    return 'text'
  }
}

function getThemeName(theme: ResolvedTheme): string {
  return theme === 'dark' ? 'github-dark-default' : 'github-light'
}

export async function highlightCodeTokens(
  code: string,
  lang: SupportedCodeLang,
  theme: ResolvedTheme,
): Promise<ThemedToken[][]> {
  const highlighter = await getHighlighter()
  const resolvedLang = await ensureLanguageLoaded(highlighter, lang)
  const result = highlighter.codeToTokens(code, {
    lang: resolvedLang,
    theme: getThemeName(theme),
  })
  return result.tokens
}

export async function highlightCodeToHtml(
  code: string,
  lang: SupportedCodeLang,
  theme: ResolvedTheme,
): Promise<string> {
  const highlighter = await getHighlighter()
  const resolvedLang = await ensureLanguageLoaded(highlighter, lang)
  return highlighter.codeToHtml(code, {
    lang: resolvedLang,
    theme: getThemeName(theme),
  })
}
