import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Check, ChevronDown, ChevronRight, Copy, FileDiff, WrapText } from 'lucide-react'
import { cn } from '@/lib/cn'

interface DiffLine {
  type: 'hunk' | 'add' | 'del' | 'context' | 'meta'
  oldNum: number | null
  newNum: number | null
  text: string
}

interface DiffFile {
  filename: string
  additions: number
  deletions: number
  raw: string
  lines: DiffLine[]
}

function parseUnifiedPatch(rawText: string): { preamble: string; files: DiffFile[] } {
  const lines = rawText.replace(/\r\n/g, '\n').split('\n')
  const preambleLines: string[] = []
  const fileBlocks: string[][] = []
  let current: string[] | null = null

  for (const line of lines) {
    if (line.startsWith('diff --git ')) {
      if (current) fileBlocks.push(current)
      current = [line]
    } else if (!current && line.startsWith('--- ')) {
      current = [line]
    } else if (current) {
      current.push(line)
    } else {
      preambleLines.push(line)
    }
  }
  if (current) fileBlocks.push(current)

  const files: DiffFile[] = fileBlocks.map((block, idx) => {
    let filename = `file-${idx + 1}`
    const first = block[0] ?? ''
    const gitMatch = first.match(/^diff --git a\/(.+?) b\/(.+)$/)
    if (gitMatch?.[2]) {
      filename = gitMatch[2]
    } else {
      const plusHeader = block.find((l) => l.startsWith('+++ '))
      if (plusHeader) {
        filename = plusHeader.replace(/^\+\+\+\s+(?:b\/)?/, '').trim()
      }
    }

    let oldLine = 0
    let newLine = 0
    let additions = 0
    let deletions = 0
    const parsedLines: DiffLine[] = []

    for (const l of block) {
      if (l.startsWith('@@')) {
        const hunkMatch = l.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/)
        if (hunkMatch) {
          oldLine = Number(hunkMatch[1])
          newLine = Number(hunkMatch[2])
        }
        parsedLines.push({ type: 'hunk', oldNum: null, newNum: null, text: l })
      } else if (
        l.startsWith('diff --git ') ||
        l.startsWith('index ') ||
        l.startsWith('--- ') ||
        l.startsWith('+++ ') ||
        l.startsWith('new file mode') ||
        l.startsWith('deleted file mode')
      ) {
        parsedLines.push({ type: 'meta', oldNum: null, newNum: null, text: l })
      } else if (l.startsWith('+')) {
        additions++
        parsedLines.push({ type: 'add', oldNum: null, newNum: newLine++, text: l })
      } else if (l.startsWith('-')) {
        deletions++
        parsedLines.push({ type: 'del', oldNum: oldLine++, newNum: null, text: l })
      } else {
        parsedLines.push({
          type: 'context',
          oldNum: oldLine > 0 ? oldLine++ : null,
          newNum: newLine > 0 ? newLine++ : null,
          text: l,
        })
      }
    }

    return {
      filename,
      additions,
      deletions,
      raw: block.join('\n'),
      lines: parsedLines,
    }
  })

  return {
    preamble: preambleLines.join('\n').trim(),
    files,
  }
}

export function DiffPatchView({ patchText }: { patchText: string }) {
  const { t } = useTranslation()
  const { preamble, files } = useMemo(() => parseUnifiedPatch(patchText), [patchText])
  const [collapsed, setCollapsed] = useState<Record<number, boolean>>({})
  const [wrap, setWrap] = useState(false)
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null)

  const handleCopyFile = async (idx: number, raw: string) => {
    try {
      await navigator.clipboard.writeText(raw)
      setCopiedIdx(idx)
      toast.success(t('mail.code.copied'))
      window.setTimeout(() => setCopiedIdx(null), 1500)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  return (
    <div className="space-y-4">
      {preamble && (
        <pre
          className="overflow-x-auto whitespace-pre-wrap break-words rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/60 p-3.5 font-mono text-[12.5px] leading-relaxed text-[var(--ink-secondary)]"
          style={{ tabSize: 4 }}
        >
          {preamble}
        </pre>
      )}

      {files.length > 0 && (
        <div className="flex items-center justify-between text-[12px] text-[var(--ink-muted)]">
          <span className="inline-flex items-center gap-1.5 font-medium text-[var(--ink-secondary)]">
            <FileDiff className="h-3.5 w-3.5 text-[var(--accent)]" />
            {t('mail.code.diffFilesCount', { count: files.length })}
          </span>
          <button
            type="button"
            onClick={() => setWrap((v) => !v)}
            className={cn(
              'inline-flex items-center gap-1 rounded px-2 py-0.5 transition-colors',
              wrap
                ? 'bg-[var(--accent-soft)] text-[var(--accent)]'
                : 'hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
            )}
          >
            <WrapText className="h-3.5 w-3.5" />
            <span>{wrap ? t('mail.code.wrapOn') : t('mail.code.wrapOff')}</span>
          </button>
        </div>
      )}

      <div className="space-y-3">
        {files.map((file, idx) => {
          const isCollapsed = Boolean(collapsed[idx])
          return (
            <div
              key={`${file.filename}-${idx}`}
              className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)]"
            >
              {/* Collapsible File Header */}
              <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border-hairline)] bg-[var(--surface-sidebar)] px-3 py-2 text-[12px]">
                <button
                  type="button"
                  onClick={() => setCollapsed((prev) => ({ ...prev, [idx]: !prev[idx] }))}
                  aria-label={isCollapsed ? t('mail.code.expandFile') : t('mail.code.collapseFile')}
                  className="flex min-w-0 items-center gap-1.5 text-left font-mono font-medium text-[var(--ink-primary)] hover:text-[var(--accent)]"
                >
                  {isCollapsed ? (
                    <ChevronRight className="h-3.5 w-3.5 shrink-0" />
                  ) : (
                    <ChevronDown className="h-3.5 w-3.5 shrink-0" />
                  )}
                  <span className="truncate">{file.filename}</span>
                </button>

                <div className="flex items-center gap-2 shrink-0">
                  <span className="font-mono text-[11.5px] font-semibold text-emerald-600 dark:text-emerald-400">
                    +{file.additions}
                  </span>
                  <span className="font-mono text-[11.5px] font-semibold text-rose-600 dark:text-rose-400">
                    -{file.deletions}
                  </span>
                  <button
                    type="button"
                    onClick={() => void handleCopyFile(idx, file.raw)}
                    className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11.5px] text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
                  >
                    {copiedIdx === idx ? (
                      <Check className="h-3 w-3 text-[var(--success)]" />
                    ) : (
                      <Copy className="h-3 w-3" />
                    )}
                    <span>{t('mail.code.copyPatch')}</span>
                  </button>
                </div>
              </div>

              {/* Diff Lines */}
              {!isCollapsed && (
                <div
                  className="mail-scroll overflow-x-auto font-mono text-[12px] leading-[1.55]"
                  style={{ tabSize: 4 }}
                >
                  <table className="w-full border-collapse">
                    <tbody>
                      {file.lines.map((l, lIdx) => {
                        if (l.type === 'meta') return null
                        return (
                          <tr
                            key={lIdx}
                            className={cn(
                              l.type === 'add' &&
                                'bg-emerald-500/12 text-emerald-900 dark:bg-emerald-500/18 dark:text-emerald-200',
                              l.type === 'del' &&
                                'bg-rose-500/12 text-rose-900 dark:bg-rose-500/18 dark:text-rose-200',
                              l.type === 'hunk' &&
                                'bg-[var(--accent-soft)]/60 text-[var(--accent)]',
                            )}
                          >
                            <td
                              aria-hidden="true"
                              className="w-9 select-none border-r border-[var(--border-hairline)] px-2 text-right tabular-nums text-[11px] text-[var(--ink-subtle)]"
                            >
                              {l.oldNum ?? ''}
                            </td>
                            <td
                              aria-hidden="true"
                              className="w-9 select-none border-r border-[var(--border-hairline)] px-2 text-right tabular-nums text-[11px] text-[var(--ink-subtle)]"
                            >
                              {l.newNum ?? ''}
                            </td>
                            <td
                              className={cn(
                                'px-3 py-0.5',
                                wrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre',
                              )}
                            >
                              {l.text || ' '}
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
