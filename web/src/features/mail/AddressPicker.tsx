import { AtSign, ChevronsUpDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AddressCounts, AddressFilter } from '@/api'
import { cn } from '@/lib/cn'
import { readAddressFilter, showAddressPicker } from './addressViews'

export function AddressPicker({
  counts,
  selection,
  collapsed = false,
  onChange,
}: {
  counts?: AddressCounts
  selection: AddressFilter
  collapsed?: boolean
  onChange: (selection: AddressFilter) => void
}) {
  const { t } = useTranslation()
  if (!counts || !showAddressPicker(counts)) return null

  const label = t('nav.mailboxAddress')
  const withUnread = (address: string, unread: number) =>
    t('nav.addressUnread', { address, count: unread })

  const activeEntry =
    selection === 'all'
      ? { display: t('nav.allAddresses'), unread: counts.all_unread }
      : selection === 'other'
        ? { display: t('nav.otherAddresses'), unread: counts.other.unread }
        : (() => {
            const found = counts.addresses.find((a) => a.id === selection)
            return found
              ? { display: found.address, unread: found.unread }
              : { display: t('nav.allAddresses'), unread: counts.all_unread }
          })()

  return (
    <div className={cn('space-y-1', collapsed ? 'px-0' : 'px-0.5')}>
      {!collapsed && (
        <div className="flex items-center justify-between px-2">
          <span className="text-[11px] font-semibold uppercase tracking-wider text-[var(--ink-muted)]">
            {label}
          </span>
        </div>
      )}

      <div
        className={cn(
          'group relative flex items-center rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] shadow-[var(--shadow-xs)] transition-colors hover:border-[var(--border-strong)] hover:bg-[var(--surface-hover)]/40 focus-within:border-[var(--accent)] focus-within:ring-2 focus-within:ring-[var(--accent-ring)]',
          collapsed ? 'mx-auto h-9 w-9 justify-center' : 'h-9 w-full gap-2 px-2.5',
        )}
      >
        <AtSign
          aria-hidden="true"
          className={cn(
            'h-3.5 w-3.5 shrink-0 transition-colors',
            selection === 'all'
              ? 'text-[var(--ink-muted)] group-hover:text-[var(--ink-primary)]'
              : 'text-[var(--accent-ink)]',
          )}
        />

        {!collapsed && (
          <>
            <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-[var(--ink-primary)]">
              {activeEntry.display}
            </span>
            {activeEntry.unread > 0 && (
              <span className="shrink-0 rounded-full bg-[var(--accent-soft)] px-1.5 py-0.5 text-[10.5px] font-semibold tabular-nums leading-none text-[var(--accent-ink)]">
                {activeEntry.unread > 999 ? '999+' : activeEntry.unread}
              </span>
            )}
            <ChevronsUpDown
              aria-hidden="true"
              className="h-3.5 w-3.5 shrink-0 text-[var(--ink-muted)]"
            />
          </>
        )}

        {collapsed && activeEntry.unread > 0 && (
          <span
            aria-hidden="true"
            className="pointer-events-none absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full bg-[var(--accent)] ring-2 ring-[var(--surface-sidebar)]"
          />
        )}

        <select
          aria-label={label}
          title={activeEntry.display}
          value={String(selection)}
          onChange={(event) => onChange(readAddressFilter(event.target.value))}
          className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
        >
          <option className="text-[var(--ink-primary)]" value="all">
            {withUnread(t('nav.allAddresses'), counts.all_unread)}
          </option>
          {counts.addresses.map(({ id, address, unread }) => (
            <option className="text-[var(--ink-primary)]" key={id} value={id}>
              {withUnread(address, unread)}
            </option>
          ))}
          {counts.other.has_mail && (
            <option className="text-[var(--ink-primary)]" value="other">
              {withUnread(t('nav.otherAddresses'), counts.other.unread)}
            </option>
          )}
        </select>
      </div>
    </div>
  )
}
