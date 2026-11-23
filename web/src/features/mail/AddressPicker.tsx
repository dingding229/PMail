import { AtSign } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AddressCounts, AddressFilter } from '@/api'
import { cn } from '@/lib/cn'
import { readAddressFilter, showAddressPicker } from './addressViews'

export function AddressPicker({ counts, selection, collapsed = false, onChange }: {
  counts?: AddressCounts
  selection: AddressFilter
  collapsed?: boolean
  onChange: (selection: AddressFilter) => void
}) {
  const { t } = useTranslation()
  if (!counts || !showAddressPicker(counts)) return null
  const label = t('nav.mailboxAddress')
  const withUnread = (address: string, unread: number) => t('nav.addressUnread', { address, count: unread })
  return (
    <div className="relative">
      {collapsed && <AtSign aria-hidden="true" className="pointer-events-none absolute left-1/2 top-1/2 z-10 h-4 w-4 -translate-x-1/2 -translate-y-1/2 text-[var(--ink-secondary)]" />}
      <select
        aria-label={label}
        title={label}
        value={String(selection)}
        onChange={(event) => onChange(readAddressFilter(event.target.value))}
        className={cn('h-9 w-full cursor-pointer rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] text-[12px] focus-visible:outline-2 focus-visible:outline-[var(--accent)]',
          collapsed ? 'appearance-none px-0 text-transparent' : 'px-2 text-[var(--ink-primary)]')}
      >
        <option className="text-[var(--ink-primary)]" value="all">{withUnread(t('nav.allAddresses'), counts.all_unread)}</option>
        {counts.addresses.map(({ id, address, unread }) => (
          <option className="text-[var(--ink-primary)]" key={id} value={id}>{withUnread(address, unread)}</option>
        ))}
        {counts.other.has_mail && <option className="text-[var(--ink-primary)]" value="other">{withUnread(t('nav.otherAddresses'), counts.other.unread)}</option>}
      </select>
    </div>
  )
}
