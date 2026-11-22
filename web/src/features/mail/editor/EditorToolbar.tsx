import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { Editor } from '@tiptap/react'
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  Bold,
  CheckSquare,
  Code,
  Code2,
  Columns2,
  Eraser,
  Heading2,
  Highlighter,
  ImagePlus,
  Italic,
  Link2,
  List,
  ListOrdered,
  Minus,
  MoreHorizontal,
  Palette,
  Quote,
  Redo2,
  Rows2,
  Strikethrough,
  Table2,
  Trash2,
  Underline as UnderlineIcon,
  Undo2,
} from 'lucide-react'
import { COMPOSE_CODE_LANGUAGES } from '@/lib/code-languages'
import { cn } from '@/lib/cn'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

interface EditorToolbarProps {
  editor: Editor | null
  isMobile: boolean
  onToggleLinkBubble: () => void
  onTriggerImageUpload: () => void
}

const FONT_SIZES = ['12px', '13px', '14px', '16px', '18px', '20px', '24px'] as const

const TEXT_COLORS = [
  '#0f172a',
  '#475569',
  '#dc2626',
  '#ea580c',
  '#ca8a04',
  '#059669',
  '#2563eb',
  '#7c3aed',
  '#db2777',
  '#f8fafc',
] as const

const HIGHLIGHT_COLORS = [
  '#fef08a',
  '#fed7aa',
  '#fecaca',
  '#bbf7d0',
  '#bfdbfe',
  '#e9d5ff',
  '#e2e8f0',
] as const

function ToolButton({
  active = false,
  disabled = false,
  title,
  onClick,
  children,
}: {
  active?: boolean
  disabled?: boolean
  title: string
  onClick?: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      title={title}
      aria-label={title}
      aria-pressed={active}
      className={cn(
        'inline-flex h-7 min-w-7 shrink-0 items-center justify-center rounded-[var(--radius-xs)] px-1 transition-colors',
        active
          ? 'bg-[var(--accent-soft)] text-[var(--accent-ink)]'
          : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
        disabled && 'pointer-events-none opacity-40',
      )}
    >
      {children}
    </button>
  )
}

function Divider() {
  return <span aria-hidden="true" className="mx-0.5 h-4 w-px shrink-0 bg-[var(--border-hairline)]" />
}

export function EditorToolbar({
  editor,
  isMobile,
  onToggleLinkBubble,
  onTriggerImageUpload,
}: EditorToolbarProps) {
  const { t } = useTranslation()
  const [colorOpen, setColorOpen] = useState(false)
  const [highlightOpen, setHighlightOpen] = useState(false)
  const [tableOpen, setTableOpen] = useState(false)
  const [moreOpen, setMoreOpen] = useState(false)

  if (!editor) return null

  const currentFontSize =
    (editor.getAttributes('textStyle').fontSize as string | undefined) || ''
  const currentColor =
    (editor.getAttributes('textStyle').color as string | undefined) || ''
  const inTable = editor.isActive('table')
  const inCodeBlock = editor.isActive('codeBlock')

  const fontSizeControl = (
    <select
      aria-label={t('compose.toolbar.fontSize')}
      title={t('compose.toolbar.fontSize')}
      value={currentFontSize}
      onChange={(e) => {
        const val = e.target.value
        if (!val) {
          editor.chain().focus().unsetFontSize().run()
        } else {
          editor.chain().focus().setFontSize(val).run()
        }
      }}
      className="h-7 shrink-0 rounded-[var(--radius-xs)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-1.5 text-[11.5px] font-medium text-[var(--ink-primary)] outline-none focus:border-[var(--accent)]"
    >
      <option value="">{t('compose.toolbar.fontSizeDefault')}</option>
      {FONT_SIZES.map((sz) => (
        <option key={sz} value={sz}>
          {sz}
        </option>
      ))}
    </select>
  )

  const colorPopover = (
    <Popover open={colorOpen} onOpenChange={setColorOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          title={t('compose.toolbar.textColor')}
          aria-label={t('compose.toolbar.textColor')}
          className={cn(
            'inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius-xs)] transition-colors',
            currentColor
              ? 'bg-[var(--accent-soft)] text-[var(--accent-ink)]'
              : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
          )}
        >
          <span className="relative flex flex-col items-center">
            <Palette className="h-3.5 w-3.5" />
            {currentColor && (
              <span
                className="mt-0.5 h-0.5 w-3 rounded-full"
                style={{ backgroundColor: currentColor }}
              />
            )}
          </span>
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-48 p-2.5">
        <p className="mb-2 text-[11px] font-medium text-[var(--ink-muted)]">
          {t('compose.toolbar.textColor')}
        </p>
        <div className="grid grid-cols-5 gap-1.5">
          {TEXT_COLORS.map((hex) => (
            <button
              key={hex}
              type="button"
              onClick={() => {
                editor.chain().focus().setColor(hex).run()
                setColorOpen(false)
              }}
              aria-label={hex}
              className={cn(
                'h-6 w-6 rounded-full border border-[var(--border-subtle)] transition-transform hover:scale-110',
                currentColor === hex && 'ring-2 ring-[var(--accent)] ring-offset-1',
              )}
              style={{ backgroundColor: hex }}
            />
          ))}
        </div>
        <button
          type="button"
          onClick={() => {
            editor.chain().focus().unsetColor().run()
            setColorOpen(false)
          }}
          className="mt-2 w-full rounded-[var(--radius-xs)] px-2 py-1 text-center text-[11.5px] text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
        >
          {t('compose.toolbar.resetColor')}
        </button>
      </PopoverContent>
    </Popover>
  )

  const highlightPopover = (
    <Popover open={highlightOpen} onOpenChange={setHighlightOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          title={t('compose.toolbar.highlight')}
          aria-label={t('compose.toolbar.highlight')}
          className={cn(
            'inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius-xs)] transition-colors',
            editor.isActive('highlight')
              ? 'bg-[var(--accent-soft)] text-[var(--accent-ink)]'
              : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
          )}
        >
          <Highlighter className="h-3.5 w-3.5" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-44 p-2.5">
        <p className="mb-2 text-[11px] font-medium text-[var(--ink-muted)]">
          {t('compose.toolbar.highlight')}
        </p>
        <div className="flex flex-wrap gap-1.5">
          {HIGHLIGHT_COLORS.map((hex) => (
            <button
              key={hex}
              type="button"
              onClick={() => {
                editor.chain().focus().toggleHighlight({ color: hex }).run()
                setHighlightOpen(false)
              }}
              aria-label={hex}
              className="h-6 w-6 rounded border border-[var(--border-subtle)] transition-transform hover:scale-110"
              style={{ backgroundColor: hex }}
            />
          ))}
        </div>
        <button
          type="button"
          onClick={() => {
            editor.chain().focus().unsetHighlight().run()
            setHighlightOpen(false)
          }}
          className="mt-2 w-full rounded-[var(--radius-xs)] px-2 py-1 text-center text-[11.5px] text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
        >
          {t('compose.toolbar.clearHighlight')}
        </button>
      </PopoverContent>
    </Popover>
  )

  const tablePopover = (
    <Popover open={tableOpen} onOpenChange={setTableOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          title={t('compose.toolbar.table')}
          aria-label={t('compose.toolbar.table')}
          className={cn(
            'inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius-xs)] transition-colors',
            inTable
              ? 'bg-[var(--accent-soft)] text-[var(--accent-ink)]'
              : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
          )}
        >
          <Table2 className="h-3.5 w-3.5" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-48 p-1.5 text-[12.5px]">
        <button
          type="button"
          onClick={() => {
            editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
            setTableOpen(false)
          }}
          className="flex w-full items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1.5 text-left text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]"
        >
          <Table2 className="h-3.5 w-3.5 text-[var(--accent-ink)]" />
          <span>{t('compose.toolbar.insertTable')}</span>
        </button>
        {inTable && (
          <>
            <div className="my-1 h-px bg-[var(--border-hairline)]" />
            <button
              type="button"
              onClick={() => {
                editor.chain().focus().addRowBefore().run()
                setTableOpen(false)
              }}
              className="flex w-full items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1.5 text-left text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]"
            >
              <Rows2 className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              <span>{t('compose.toolbar.addRowBefore')}</span>
            </button>
            <button
              type="button"
              onClick={() => {
                editor.chain().focus().addRowAfter().run()
                setTableOpen(false)
              }}
              className="flex w-full items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1.5 text-left text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]"
            >
              <Rows2 className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              <span>{t('compose.toolbar.addRowAfter')}</span>
            </button>
            <button
              type="button"
              onClick={() => {
                editor.chain().focus().deleteRow().run()
                setTableOpen(false)
              }}
              className="flex w-full items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1.5 text-left text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]"
            >
              <Trash2 className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              <span>{t('compose.toolbar.deleteRow')}</span>
            </button>
            <div className="my-1 h-px bg-[var(--border-hairline)]" />
            <button
              type="button"
              onClick={() => {
                editor.chain().focus().addColumnBefore().run()
                setTableOpen(false)
              }}
              className="flex w-full items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1.5 text-left text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]"
            >
              <Columns2 className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              <span>{t('compose.toolbar.addColBefore')}</span>
            </button>
            <button
              type="button"
              onClick={() => {
                editor.chain().focus().addColumnAfter().run()
                setTableOpen(false)
              }}
              className="flex w-full items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1.5 text-left text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]"
            >
              <Columns2 className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              <span>{t('compose.toolbar.addColAfter')}</span>
            </button>
            <button
              type="button"
              onClick={() => {
                editor.chain().focus().deleteColumn().run()
                setTableOpen(false)
              }}
              className="flex w-full items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1.5 text-left text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]"
            >
              <Trash2 className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              <span>{t('compose.toolbar.deleteCol')}</span>
            </button>
            <div className="my-1 h-px bg-[var(--border-hairline)]" />
            <button
              type="button"
              onClick={() => {
                editor.chain().focus().deleteTable().run()
                setTableOpen(false)
              }}
              className="flex w-full items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1.5 text-left text-[var(--danger)] hover:bg-[var(--danger-soft)]"
            >
              <Trash2 className="h-3.5 w-3.5" />
              <span>{t('compose.toolbar.deleteTable')}</span>
            </button>
          </>
        )}
      </PopoverContent>
    </Popover>
  )

  const alignButtons = (
    <>
      <ToolButton
        active={editor.isActive({ textAlign: 'left' })}
        title={t('compose.toolbar.alignLeft')}
        onClick={() => editor.chain().focus().setTextAlign('left').run()}
      >
        <AlignLeft className="h-3.5 w-3.5" />
      </ToolButton>
      <ToolButton
        active={editor.isActive({ textAlign: 'center' })}
        title={t('compose.toolbar.alignCenter')}
        onClick={() => editor.chain().focus().setTextAlign('center').run()}
      >
        <AlignCenter className="h-3.5 w-3.5" />
      </ToolButton>
      <ToolButton
        active={editor.isActive({ textAlign: 'right' })}
        title={t('compose.toolbar.alignRight')}
        onClick={() => editor.chain().focus().setTextAlign('right').run()}
      >
        <AlignRight className="h-3.5 w-3.5" />
      </ToolButton>
    </>
  )

  return (
    <div className="relative shrink-0 border-b border-[var(--border-hairline)] bg-[var(--surface-sidebar)]/55">
      <div
        role="toolbar"
        aria-label={t('compose.toolbar.ariaLabel')}
        className={cn(
          'flex items-center gap-0.5 px-2.5 py-1.5',
          isMobile ? 'no-scrollbar flex-nowrap overflow-x-auto pr-8' : 'flex-wrap',
        )}
      >
        {/* Font Size */}
        {!isMobile && (
          <>
            {fontSizeControl}
            <Divider />
          </>
        )}

        {/* Basic Text Marks */}
        <ToolButton
          active={editor.isActive('bold')}
          title={t('compose.toolbar.bold')}
          onClick={() => editor.chain().focus().toggleBold().run()}
        >
          <Bold className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          active={editor.isActive('italic')}
          title={t('compose.toolbar.italic')}
          onClick={() => editor.chain().focus().toggleItalic().run()}
        >
          <Italic className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          active={editor.isActive('underline')}
          title={t('compose.toolbar.underline')}
          onClick={() => editor.chain().focus().toggleUnderline().run()}
        >
          <UnderlineIcon className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          active={editor.isActive('strike')}
          title={t('compose.toolbar.strike')}
          onClick={() => editor.chain().focus().toggleStrike().run()}
        >
          <Strikethrough className="h-3.5 w-3.5" />
        </ToolButton>

        {!isMobile && (
          <>
            {colorPopover}
            {highlightPopover}
          </>
        )}

        <Divider />

        {/* Heading & Lists */}
        <ToolButton
          active={editor.isActive('heading', { level: 2 })}
          title={t('compose.toolbar.heading')}
          onClick={() => editor.chain().focus().toggleHeading({ level: 2 }).run()}
        >
          <Heading2 className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          active={editor.isActive('bulletList')}
          title={t('compose.toolbar.bulletList')}
          onClick={() => editor.chain().focus().toggleBulletList().run()}
        >
          <List className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          active={editor.isActive('orderedList')}
          title={t('compose.toolbar.orderedList')}
          onClick={() => editor.chain().focus().toggleOrderedList().run()}
        >
          <ListOrdered className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          active={editor.isActive('taskList')}
          title={t('compose.toolbar.taskList')}
          onClick={() => editor.chain().focus().toggleTaskList().run()}
        >
          <CheckSquare className="h-3.5 w-3.5" />
        </ToolButton>

        {!isMobile && (
          <>
            <Divider />
            {alignButtons}
          </>
        )}

        <Divider />

        {/* Quote, Code, Code Block (+ Language Selector) */}
        <ToolButton
          active={editor.isActive('blockquote')}
          title={t('compose.toolbar.quote')}
          onClick={() => editor.chain().focus().toggleBlockquote().run()}
        >
          <Quote className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          active={editor.isActive('code')}
          title={t('compose.toolbar.code')}
          onClick={() => editor.chain().focus().toggleCode().run()}
        >
          <Code className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          active={inCodeBlock}
          title={t('compose.toolbar.codeBlock')}
          onClick={() => editor.chain().focus().toggleCodeBlock().run()}
        >
          <Code2 className="h-3.5 w-3.5" />
        </ToolButton>

        {inCodeBlock && (
          <select
            aria-label={t('compose.toolbar.codeLanguage')}
            title={t('compose.toolbar.codeLanguage')}
            value={(editor.getAttributes('codeBlock').language as string | undefined) || ''}
            onChange={(e) => {
              editor
                .chain()
                .focus()
                .updateAttributes('codeBlock', { language: e.target.value || null })
                .run()
            }}
            className="mx-0.5 h-7 shrink-0 rounded-[var(--radius-xs)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-1.5 text-[11.5px] font-medium text-[var(--ink-primary)] outline-none focus:border-[var(--accent)]"
          >
            <option value="">{t('compose.toolbar.codeLanguageAuto')}</option>
            {COMPOSE_CODE_LANGUAGES.map((lang) => (
              <option key={lang} value={lang}>
                {t(`mail.code.languages.${lang}` as const)}
              </option>
            ))}
          </select>
        )}

        <Divider />

        {/* Link, Image (C9 reserved), Table, HR, Clear Format */}
        <ToolButton
          active={editor.isActive('link')}
          title={t('compose.toolbar.link')}
          onClick={onToggleLinkBubble}
        >
          <Link2 className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          title={t('compose.toolbar.insertImage')}
          onClick={onTriggerImageUpload}
        >
          <ImagePlus className="h-3.5 w-3.5" />
        </ToolButton>

        {!isMobile && (
          <>
            {tablePopover}
            <ToolButton
              title={t('compose.toolbar.horizontalRule')}
              onClick={() => editor.chain().focus().setHorizontalRule().run()}
            >
              <Minus className="h-3.5 w-3.5" />
            </ToolButton>
            <ToolButton
              title={t('compose.toolbar.clearFormat')}
              onClick={() => editor.chain().focus().unsetAllMarks().clearNodes().run()}
            >
              <Eraser className="h-3.5 w-3.5" />
            </ToolButton>
          </>
        )}

        {/* Mobile "More" Overflow Popover */}
        {isMobile && (
          <Popover open={moreOpen} onOpenChange={setMoreOpen}>
            <PopoverTrigger asChild>
              <button
                type="button"
                title={t('compose.toolbar.moreTools')}
                aria-label={t('compose.toolbar.moreTools')}
                className="inline-flex h-7 shrink-0 items-center gap-1 rounded-[var(--radius-xs)] px-2 text-[11.5px] font-medium text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
                <span>{t('compose.toolbar.moreTools')}</span>
              </button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64 space-y-2.5 p-3">
              <div className="flex items-center justify-between gap-2">
                <span className="text-[11.5px] text-[var(--ink-muted)]">
                  {t('compose.toolbar.fontSize')}
                </span>
                {fontSizeControl}
              </div>
              <div className="flex flex-wrap items-center gap-1 border-t border-[var(--border-hairline)] pt-2">
                {colorPopover}
                {highlightPopover}
                <Divider />
                {alignButtons}
                <Divider />
                {tablePopover}
                <ToolButton
                  title={t('compose.toolbar.horizontalRule')}
                  onClick={() => {
                    editor.chain().focus().setHorizontalRule().run()
                    setMoreOpen(false)
                  }}
                >
                  <Minus className="h-3.5 w-3.5" />
                </ToolButton>
                <ToolButton
                  title={t('compose.toolbar.clearFormat')}
                  onClick={() => {
                    editor.chain().focus().unsetAllMarks().clearNodes().run()
                    setMoreOpen(false)
                  }}
                >
                  <Eraser className="h-3.5 w-3.5" />
                </ToolButton>
              </div>
            </PopoverContent>
          </Popover>
        )}

        <Divider />

        {/* Undo / Redo */}
        <ToolButton
          disabled={!editor.can().undo()}
          title={t('compose.toolbar.undo')}
          onClick={() => editor.chain().focus().undo().run()}
        >
          <Undo2 className="h-3.5 w-3.5" />
        </ToolButton>
        <ToolButton
          disabled={!editor.can().redo()}
          title={t('compose.toolbar.redo')}
          onClick={() => editor.chain().focus().redo().run()}
        >
          <Redo2 className="h-3.5 w-3.5" />
        </ToolButton>
      </div>

      {/* Subtle right fade hint on mobile horizontal scroll toolbar */}
      {isMobile && (
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-y-0 right-0 w-7 bg-gradient-to-l from-[var(--surface-canvas)] to-transparent"
        />
      )}
    </div>
  )
}
