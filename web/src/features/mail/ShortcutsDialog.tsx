import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface ShortcutsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function Kbd({ children }: { children: string }) {
  return (
    <kbd className="inline-flex min-w-[24px] items-center justify-center rounded-[5px] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-1.5 py-0.5 font-mono text-[11.5px] font-medium text-[var(--ink-primary)] shadow-[0_1px_0_var(--border-subtle)]">
      {children}
    </kbd>
  )
}

export function ShortcutsDialog({ open, onOpenChange }: ShortcutsDialogProps) {
  const { t } = useTranslation()

  const sections = [
    {
      title: t('shortcuts.sections.navigation'),
      items: [
        { keys: ['j', '↓'], label: t('shortcuts.items.next') },
        { keys: ['k', '↑'], label: t('shortcuts.items.prev') },
        { keys: ['Enter', 'o'], label: t('shortcuts.items.open') },
        { keys: ['/'], label: t('shortcuts.items.search') },
      ],
    },
    {
      title: t('shortcuts.sections.actions'),
      items: [
        { keys: ['r'], label: t('shortcuts.items.reply') },
        { keys: ['a'], label: t('shortcuts.items.replyAll') },
        { keys: ['f'], label: t('shortcuts.items.forward') },
        { keys: ['s'], label: t('shortcuts.items.star') },
        { keys: ['e'], label: t('shortcuts.items.archive') },
        { keys: ['#'], label: t('shortcuts.items.delete') },
        { keys: ['z', 'u'], label: t('shortcuts.items.restore') },
        { keys: ['Shift', 'C'], label: t('shortcuts.items.copyCode') },
      ],
    },
    {
      title: t('shortcuts.sections.compose'),
      items: [
        { keys: ['c'], label: t('shortcuts.items.compose') },
        { keys: ['⌘/Ctrl', 'Enter'], label: t('shortcuts.items.send') },
        { keys: ['Esc'], label: t('shortcuts.items.escape') },
        { keys: ['?'], label: t('shortcuts.items.help') },
      ],
    },
  ]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('shortcuts.title')}</DialogTitle>
          <DialogDescription>{t('shortcuts.subtitle')}</DialogDescription>
        </DialogHeader>

        <div className="mt-4 grid grid-cols-1 gap-5 sm:grid-cols-2">
          {sections.map((section) => (
            <div key={section.title} className="space-y-2">
              <h3 className="text-[11.5px] font-semibold uppercase tracking-wider text-[var(--ink-muted)]">
                {section.title}
              </h3>
              <ul className="space-y-1.5">
                {section.items.map((item) => (
                  <li
                    key={item.label}
                    className="flex items-center justify-between gap-2 py-0.5 text-[13px]"
                  >
                    <span className="text-[var(--ink-secondary)]">{item.label}</span>
                    <span className="flex items-center gap-1 shrink-0">
                      {item.keys.map((k) => (
                        <Kbd key={k}>{k}</Kbd>
                      ))}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
