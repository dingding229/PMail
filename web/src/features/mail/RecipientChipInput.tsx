import { useMemo, useRef, useState } from 'react'
import { X } from 'lucide-react'
import { isValidEmail } from '@/lib/format'
import { cn } from '@/lib/cn'

export interface ContactSuggestion {
  name: string
  email: string
}

interface RecipientChipInputProps {
  id: string
  values: string[]
  onChange: (next: string[]) => void
  suggestions: ContactSuggestion[]
  placeholder: string
  autoFocus?: boolean
}

export function RecipientChipInput({
  id,
  values,
  onChange,
  suggestions,
  placeholder,
  autoFocus = false,
}: RecipientChipInputProps) {
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [draft, setDraft] = useState('')
  const [open, setOpen] = useState(false)
  const [highlightIdx, setHighlightIdx] = useState(0)

  const filteredSuggestions = useMemo(() => {
    const q = draft.trim().toLowerCase()
    const existing = new Set(values.map((v) => v.toLowerCase()))
    return suggestions
      .filter((s) => !existing.has(s.email.toLowerCase()))
      .filter((s) => {
        if (!q) return false
        return s.email.toLowerCase().includes(q) || s.name.toLowerCase().includes(q)
      })
      .slice(0, 6)
  }, [draft, suggestions, values])

  const commitTokens = (raw: string) => {
    const tokens = raw
      .split(/[,;\n\s]+/)
      .map((s) => s.trim())
      .filter(Boolean)
    if (tokens.length === 0) return

    const existingLower = new Set(values.map((v) => v.toLowerCase()))
    const added: string[] = []
    for (const token of tokens) {
      const lower = token.toLowerCase()
      if (!existingLower.has(lower)) {
        existingLower.add(lower)
        added.push(token)
      }
    }
    if (added.length > 0) {
      onChange([...values, ...added])
    }
    setDraft('')
    setOpen(false)
    setHighlightIdx(0)
  }

  const removeAt = (index: number) => {
    onChange(values.filter((_, i) => i !== index))
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (open && filteredSuggestions.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setHighlightIdx((i) => (i + 1) % filteredSuggestions.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setHighlightIdx((i) => (i - 1 + filteredSuggestions.length) % filteredSuggestions.length)
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        const picked = filteredSuggestions[highlightIdx]
        if (picked) {
          e.preventDefault()
          commitTokens(picked.email)
          return
        }
      }
    }

    if (e.key === 'Enter' || e.key === ',' || e.key === ';' || e.key === 'Tab') {
      if (draft.trim()) {
        e.preventDefault()
        commitTokens(draft)
      }
    } else if (e.key === 'Backspace' && !draft && values.length > 0) {
      e.preventDefault()
      removeAt(values.length - 1)
    } else if (e.key === 'Escape') {
      setOpen(false)
    }
  }

  return (
    <div className="relative flex-1 min-w-0">
      <div
        onClick={() => inputRef.current?.focus()}
        className="flex min-h-[34px] flex-wrap items-center gap-1.5 py-1 cursor-text"
      >
        {values.map((addr, idx) => {
          const valid = isValidEmail(addr)
          return (
            <span
              key={`${addr}-${idx}`}
              className={cn(
                'inline-flex max-w-full items-center gap-1 rounded-full px-2.5 py-0.5 text-[12.5px] leading-tight transition-colors',
                valid
                  ? 'bg-[var(--surface-subtle)] text-[var(--ink-primary)] border border-[var(--border-hairline)]'
                  : 'bg-[var(--danger-soft)] text-[var(--danger)] border border-[var(--danger)]/40',
              )}
            >
              <span className="truncate max-w-[220px]">{addr}</span>
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation()
                  removeAt(idx)
                }}
                aria-label={addr}
                className="rounded-full p-0.5 hover:bg-black/10 dark:hover:bg-white/10"
              >
                <X className="h-3 w-3" />
              </button>
            </span>
          )
        })}

        <input
          ref={inputRef}
          id={id}
          type="text"
          value={draft}
          autoFocus={autoFocus}
          onChange={(e) => {
            setDraft(e.target.value)
            setOpen(true)
            setHighlightIdx(0)
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => {
            window.setTimeout(() => {
              if (draft.trim()) commitTokens(draft)
              setOpen(false)
            }, 140)
          }}
          onPaste={(e) => {
            const pasted = e.clipboardData.getData('text')
            if (/[,;\n\s]/.test(pasted)) {
              e.preventDefault()
              commitTokens(pasted)
            }
          }}
          onKeyDown={handleKeyDown}
          placeholder={values.length === 0 ? placeholder : ''}
          className="min-w-[140px] flex-1 bg-transparent text-[13.5px] text-[var(--ink-primary)] placeholder:text-[var(--ink-subtle)] outline-none"
        />
      </div>

      {/* Autocomplete dropdown */}
      {open && filteredSuggestions.length > 0 && (
        <ul
          role="listbox"
          className="absolute left-0 right-0 top-full z-30 mt-1 max-h-52 overflow-y-auto rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-1 shadow-[var(--shadow-md)]"
        >
          {filteredSuggestions.map((item, idx) => (
            <li key={item.email}>
              <button
                type="button"
                onMouseDown={(e) => {
                  e.preventDefault()
                  commitTokens(item.email)
                }}
                className={cn(
                  'flex w-full items-center justify-between gap-2 rounded-[var(--radius-xs)] px-2.5 py-1.5 text-left text-[13px] transition-colors',
                  idx === highlightIdx
                    ? 'bg-[var(--surface-active)] text-[var(--accent)]'
                    : 'text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]',
                )}
              >
                <span className="truncate font-medium">{item.name || item.email}</span>
                {item.name && (
                  <span className="truncate text-[12px] text-[var(--ink-muted)]">{item.email}</span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
