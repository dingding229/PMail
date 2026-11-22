/**
 * Converts TipTap rich-text HTML into a clean, structured plain-text alternative
 * (`text/plain`) suitable for RFC 5322 multipart/alternative email delivery.
 *
 * Properly formats:
 * - Tables -> Markdown-style aligned pipe tables (`| A | B |`)
 * - Lists -> `- item` (unordered), `1. item` (ordered), `[x]/[ ] item` (task lists)
 * - Links -> `label (https://...)`
 * - Code blocks -> fenced ```lang ... ```
 * - Blockquotes -> `> ...`
 * - Horizontal rules -> `---`
 * - Images -> `[alt]` / `[Image]`
 */

function normalizeInlineText(str: string): string {
  return str.replace(/\u00a0/g, ' ').replace(/[ \t]+/g, ' ')
}

function renderInlineChildren(node: Node): string {
  let out = ''
  node.childNodes.forEach((child) => {
    if (child.nodeType === Node.TEXT_NODE) {
      out += normalizeInlineText(child.textContent ?? '')
      return
    }
    if (child.nodeType !== Node.ELEMENT_NODE) return
    const el = child as HTMLElement
    const tag = el.tagName.toLowerCase()

    if (tag === 'br') {
      out += '\n'
    } else if (tag === 'a') {
      const label = renderInlineChildren(el).trim()
      const href = (el.getAttribute('href') || '').trim()
      if (!href || href === label || href === `mailto:${label}`) {
        out += label || href
      } else if (!label) {
        out += href
      } else {
        out += `${label} (${href})`
      }
    } else if (tag === 'code') {
      out += `\`${el.textContent ?? ''}\``
    } else if (tag === 'img') {
      const alt = (el.getAttribute('alt') || el.getAttribute('data-filename') || '').trim()
      out += alt ? `[${alt}]` : '[Image]'
    } else {
      out += renderInlineChildren(el)
    }
  })
  return out
}

function renderTable(table: HTMLElement): string {
  const rows: string[][] = []
  let hasHeaderRow = false

  table.querySelectorAll('tr').forEach((tr, rowIdx) => {
    const cells: string[] = []
    let allTh = true
    tr.querySelectorAll('th, td').forEach((cell) => {
      if (cell.tagName.toLowerCase() !== 'th') allTh = false
      const cellText = renderInlineChildren(cell)
        .replace(/\r?\n/g, ' ')
        .replace(/\|/g, '\\|')
        .trim()
      cells.push(cellText)
    })
    if (cells.length > 0) {
      if (rowIdx === 0 && allTh) hasHeaderRow = true
      rows.push(cells)
    }
  })

  if (rows.length === 0) return ''
  const colCount = Math.max(...rows.map((r) => r.length))
  const colWidths = Array.from({ length: colCount }, (_, colIdx) =>
    Math.max(3, ...rows.map((r) => (r[colIdx] ?? '').length)),
  )

  const formatRow = (r: string[]) =>
    `| ${Array.from({ length: colCount }, (_, i) => (r[i] ?? '').padEnd(colWidths[i]!)).join(' | ')} |`

  const lines: string[] = []
  rows.forEach((r, idx) => {
    lines.push(formatRow(r))
    if (idx === 0 && (hasHeaderRow || rows.length > 1)) {
      lines.push(`| ${colWidths.map((w) => '-'.repeat(w)).join(' | ')} |`)
    }
  })

  return lines.join('\n')
}

function renderList(listEl: HTMLElement, depth = 0): string {
  const tag = listEl.tagName.toLowerCase()
  const isTaskList = listEl.getAttribute('data-type') === 'taskList'
  const indent = '  '.repeat(depth)
  const lines: string[] = []
  let index = 1

  Array.from(listEl.children).forEach((child) => {
    if (child.tagName.toLowerCase() !== 'li') return
    const li = child as HTMLElement

    let prefix = '- '
    if (isTaskList || li.getAttribute('data-type') === 'taskItem') {
      const checkedAttr = li.getAttribute('data-checked')
      const checkbox = li.querySelector('input[type="checkbox"]') as HTMLInputElement | null
      const checked = checkedAttr === 'true' || Boolean(checkbox?.checked)
      prefix = checked ? '[x] ' : '[ ] '
    } else if (tag === 'ol') {
      prefix = `${index++}. `
    }

    const itemTextParts: string[] = []
    const nestedLists: string[] = []

    li.childNodes.forEach((sub) => {
      if (sub.nodeType === Node.TEXT_NODE) {
        const t = normalizeInlineText(sub.textContent ?? '').trim()
        if (t) itemTextParts.push(t)
        return
      }
      if (sub.nodeType !== Node.ELEMENT_NODE) return
      const subEl = sub as HTMLElement
      const subTag = subEl.tagName.toLowerCase()
      if (subTag === 'label') return // TipTap taskItem checkbox wrapper
      if (subTag === 'ul' || subTag === 'ol') {
        nestedLists.push(renderList(subEl, depth + 1))
      } else if (subTag === 'div') {
        // TipTap taskItem wraps content in <div><p>...</p></div>
        subEl.childNodes.forEach((inner) => {
          if (inner.nodeType === Node.ELEMENT_NODE) {
            const innerEl = inner as HTMLElement
            const innerTag = innerEl.tagName.toLowerCase()
            if (innerTag === 'ul' || innerTag === 'ol') {
              nestedLists.push(renderList(innerEl, depth + 1))
            } else {
              const t = renderInlineChildren(innerEl).trim()
              if (t) itemTextParts.push(t)
            }
          } else if (inner.nodeType === Node.TEXT_NODE) {
            const t = normalizeInlineText(inner.textContent ?? '').trim()
            if (t) itemTextParts.push(t)
          }
        })
      } else {
        const t = renderInlineChildren(subEl).trim()
        if (t) itemTextParts.push(t)
      }
    })

    lines.push(`${indent}${prefix}${itemTextParts.join(' ').trim()}`)
    for (const nested of nestedLists) {
      if (nested) lines.push(nested)
    }
  })

  return lines.join('\n')
}

function renderBlock(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) {
    return normalizeInlineText(node.textContent ?? '').trim()
  }
  if (node.nodeType !== Node.ELEMENT_NODE) return ''
  const el = node as HTMLElement
  const tag = el.tagName.toLowerCase()

  if (tag === 'hr') return '---'

  if (tag === 'pre') {
    const codeEl = el.querySelector('code')
    const cls = codeEl?.getAttribute('class') || el.getAttribute('class') || ''
    const langMatch = cls.match(/\b(?:language|lang)-([a-zA-Z0-9_+#.-]{1,32})\b/)
    const lang = langMatch?.[1] ?? ''
    const raw = (codeEl?.textContent ?? el.textContent ?? '').replace(/\r\n/g, '\n').replace(/\n$/, '')
    return `\`\`\`${lang}\n${raw}\n\`\`\``
  }

  if (tag === 'table') {
    return renderTable(el)
  }

  if (tag === 'ul' || tag === 'ol') {
    return renderList(el, 0)
  }

  if (tag === 'blockquote') {
    const inner = renderBlocks(el)
    if (!inner) return ''
    return inner
      .split('\n')
      .map((line) => (line ? `> ${line}` : '>'))
      .join('\n')
  }

  if (/^h[1-6]$/.test(tag)) {
    return renderInlineChildren(el).trim()
  }

  if (tag === 'p') {
    return renderInlineChildren(el).trim()
  }

  if (tag === 'div' || tag === 'section' || tag === 'article') {
    if (el.hasAttribute('data-pmail-signature')) {
      const sigBody = renderBlocks(el)
      return sigBody ? `-- \n${sigBody}` : ''
    }
    return renderBlocks(el)
  }

  return renderInlineChildren(el).trim()
}

function renderBlocks(container: Node): string {
  const parts: string[] = []
  container.childNodes.forEach((child) => {
    const rendered = renderBlock(child)
    if (rendered) parts.push(rendered)
  })
  return parts.join('\n\n')
}

export function htmlToMailPlainText(html: string): string {
  if (!html || !html.trim()) return ''
  if (typeof DOMParser === 'undefined') {
    return html.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }
  const doc = new DOMParser().parseFromString(html, 'text/html')
  return renderBlocks(doc.body)
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}
