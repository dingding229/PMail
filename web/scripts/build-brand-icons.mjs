import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { getIconData, iconToSVG } from '@iconify/utils'
import { BRAND_REGISTRY } from './brand-registry.mjs'

const require = createRequire(import.meta.url)
const rootDir = path.resolve(import.meta.dirname, '..')
const outFile = path.join(rootDir, 'src/generated/brandIcons.ts')

const ICONIFY_SETS = {
  logos: require('@iconify-json/logos/icons.json'),
  'simple-icons': require('@iconify-json/simple-icons/icons.json'),
  ri: require('@iconify-json/ri/icons.json'),
  'fa6-brands': require('@iconify-json/fa6-brands/icons.json'),
}

const LOBEHUB_ICONS_DIR = path.join(
  path.dirname(require.resolve('@lobehub/icons-static-svg/package.json')),
  'icons',
)

const ALLOWED_TAGS = new Set([
  'svg',
  'g',
  'path',
  'circle',
  'rect',
  'polygon',
  'ellipse',
  'lineargradient',
  'radialgradient',
  'stop',
  'defs',
  'clippath',
  'mask',
])

const COLOR_SETS = new Set(['logos', 'lobehub'])
// Prefer compact monochrome fallback when a colorful SVG exceeds this threshold
const MAX_COLOR_SVG_BYTES = 950

/**
 * Sanitize and namespace an SVG string at build time:
 * - Strip <script>, <foreignObject>, <style>, <title>, <desc>, comments, XML prolog
 * - Strip all on* event attributes and external href / xlink:href
 * - Allow ONLY whitelisted SVG tags
 * - Prefix all id / url(#...) / href="#..." with `bi-${brandId}-`
 */
export function sanitizeBrandSvg(rawSvg, brandId, isMonochrome) {
  let svg = String(rawSvg)
    .replace(/<\?xml[\s\S]*?\?>/gi, '')
    .replace(/<!DOCTYPE[\s\S]*?>/gi, '')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/<script[\s\S]*?<\/script>/gi, '')
    .replace(/<foreignObject[\s\S]*?<\/foreignObject>/gi, '')
    .replace(/<style[\s\S]*?<\/style>/gi, '')
    .replace(/<title[\s\S]*?<\/title>/gi, '')
    .replace(/<desc[\s\S]*?<\/desc>/gi, '')
    .trim()

  // Strip event handlers (on*)
  svg = svg.replace(/\s+on[a-z0-9_-]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi, '')

  // Strip external href / xlink:href (only allow internal #id references)
  svg = svg.replace(
    /\s+(?:xlink:)?href\s*=\s*(?:"([^"]*)"|'([^']*)')/gi,
    (match, dq, sq) => {
      const val = (dq ?? sq ?? '').trim()
      if (val.startsWith('#')) return match
      return ''
    },
  )

  // Remove width/height/style/class on root <svg> while preserving viewBox
  svg = svg.replace(/<svg\b([^>]*)>/i, (_full, attrs) => {
    const cleanedAttrs = attrs
      .replace(/\s+(?:width|height|style|class|xmlns:xlink)\s*=\s*(?:"[^"]*"|'[^']*')/gi, '')
      .trim()
    const fillAttr = isMonochrome && !/\bfill=/i.test(cleanedAttrs) ? ' fill="currentColor"' : ''
    return `<svg ${cleanedAttrs}${fillAttr}>`.replace(/\s+>/, '>')
  })

  // Collect and namespace all id="..." values to prevent cross-icon collisions
  const prefix = `bi-${brandId}-`
  const idMap = new Map()
  svg = svg.replace(/\bid\s*=\s*(?:"([^"]+)"|'([^']+)')/gi, (_m, dq, sq) => {
    const orig = (dq ?? sq).trim()
    const next = orig.startsWith(prefix) ? orig : `${prefix}${orig}`
    idMap.set(orig, next)
    return `id="${next}"`
  })

  if (idMap.size > 0) {
    svg = svg.replace(/url\(\s*['"]?#([^)'"]+)['"]?\s*\)/gi, (m, ref) => {
      const mapped = idMap.get(ref.trim())
      return mapped ? `url(#${mapped})` : m
    })
    svg = svg.replace(/\b((?:xlink:)?href)\s*=\s*(?:"(#([^"]+))"|'(#([^']+))')/gi, (m, attr, _dqFull, dqId, _sqFull, sqId) => {
      const ref = (dqId ?? sqId ?? '').trim()
      const mapped = idMap.get(ref)
      return mapped ? `${attr}="#${mapped}"` : m
    })
  }

  // Collapse whitespace cleanly
  svg = svg.replace(/>\s+</g, '><').replace(/\s{2,}/g, ' ').trim()

  // Strict tag allowlist verification
  const tagMatches = svg.matchAll(/<\/?([a-zA-Z0-9:-]+)/g)
  for (const match of tagMatches) {
    const tag = match[1].toLowerCase()
    if (!ALLOWED_TAGS.has(tag)) {
      throw new Error(`Disallowed SVG tag <${match[1]}> in brand "${brandId}"`)
    }
  }

  return svg
}

function loadCandidateSvg(candidate, brandId) {
  const { set, name } = candidate
  const isMono = !COLOR_SETS.has(set)

  if (set === 'lobehub') {
    const svgPath = path.join(LOBEHUB_ICONS_DIR, `${name}.svg`)
    if (!fs.existsSync(svgPath)) return null
    const raw = fs.readFileSync(svgPath, 'utf8')
    const sanitized = sanitizeBrandSvg(raw, brandId, false)
    return { svg: sanitized, mode: 'color', source: 'lobehub', iconName: name }
  }

  const collection = ICONIFY_SETS[set]
  if (!collection) return null
  const iconData = getIconData(collection, name)
  if (!iconData) return null

  const rendered = iconToSVG(iconData, { height: 'auto' })
  const attrs = Object.entries(rendered.attributes)
    .filter(([k]) => k !== 'width' && k !== 'height')
    .map(([k, v]) => `${k}="${String(v)}"`)
    .join(' ')
  const raw = `<svg xmlns="http://www.w3.org/2000/svg" ${attrs}>${rendered.body}</svg>`
  const sanitized = sanitizeBrandSvg(raw, brandId, isMono)
  return {
    svg: sanitized,
    mode: isMono ? 'mono' : 'color',
    source: set,
    iconName: name,
  }
}

function resolveBrandIcon(brand) {
  let oversizedColorFallback = null

  for (const candidate of brand.candidates ?? []) {
    try {
      const resolved = loadCandidateSvg(candidate, brand.id)
      if (!resolved) continue
      if (resolved.mode === 'color' && Buffer.byteLength(resolved.svg, 'utf8') > MAX_COLOR_SVG_BYTES) {
        if (!oversizedColorFallback) oversizedColorFallback = resolved
        continue
      }
      return resolved
    } catch {
      // Skip candidates containing disallowed tags (e.g. raster/foreignObject) and try next
      continue
    }
  }

  if (oversizedColorFallback) return oversizedColorFallback

  return {
    svg: undefined,
    mode: 'monogram',
    source: 'monogram',
    iconName: undefined,
  }
}

const resolvedBrands = []
const sourceCounts = {
  logos: 0,
  lobehub: 0,
  'simple-icons': 0,
  ri: 0,
  'fa6-brands': 0,
  monogram: 0,
}

let totalSvgBytes = 0

for (const brand of BRAND_REGISTRY) {
  const icon = resolveBrandIcon(brand)
  sourceCounts[icon.source] = (sourceCounts[icon.source] ?? 0) + 1
  if (icon.svg) totalSvgBytes += Buffer.byteLength(icon.svg, 'utf8')

  resolvedBrands.push({
    id: brand.id,
    name: brand.name,
    domains: brand.domains,
    color: brand.color,
    ...(brand.darkLight ? { darkLight: true } : {}),
    monogram: brand.monogram || brand.name.slice(0, 1).toUpperCase(),
    source: icon.source,
    mode: icon.mode,
    ...(icon.svg ? { svg: icon.svg } : {}),
  })
}

const usedLibraries = Object.entries(sourceCounts)
  .filter(([key, count]) => key !== 'monogram' && count > 0)
  .map(([key]) => key)

const fileContent = `// AUTO-GENERATED by scripts/build-brand-icons.mjs — DO NOT EDIT MANUALLY.
// Contains build-time sanitized SVG icons extracted from local devDependencies.

export type BrandIconSource =
  | 'logos'
  | 'lobehub'
  | 'simple-icons'
  | 'ri'
  | 'fa6-brands'
  | 'monogram'

export type BrandIconMode = 'color' | 'mono' | 'monogram'

export interface BrandIconEntry {
  id: string
  name: string
  domains: string[]
  color: string
  darkLight?: boolean
  monogram: string
  source: BrandIconSource
  mode: BrandIconMode
  svg?: string
}

export const USED_ICON_LIBRARIES: readonly string[] = ${JSON.stringify(usedLibraries)}

export const BRAND_ICON_CATALOG: readonly BrandIconEntry[] = ${JSON.stringify(resolvedBrands, null, 2)}
`

fs.mkdirSync(path.dirname(outFile), { recursive: true })
fs.writeFileSync(outFile, fileContent, 'utf8')

console.log(
  `✅ Generated ${path.relative(rootDir, outFile)}: ${resolvedBrands.length} brands, ${(totalSvgBytes / 1024).toFixed(1)} KB SVG payload.`,
)
console.log('   Sources:', sourceCounts)
