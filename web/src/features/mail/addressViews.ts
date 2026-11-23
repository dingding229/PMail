import type { AddressCounts, AddressFilter, MailboxAddressMatch } from '@/api'
import type { LocalDraft } from './types'

export function readAddressFilter(raw?: string | null): AddressFilter {
  if (raw === 'other') return raw
  if (raw && /^[1-9]\d*$/.test(raw)) {
    const id = Number(raw)
    if (Number.isSafeInteger(id)) return id
  }
  return 'all'
}

export function availableAddressFilter(requested: AddressFilter, counts?: AddressCounts): AddressFilter {
  if (!counts || requested === 'all') return requested
  if (!showAddressPicker(counts)) return 'all'
  if (requested === 'other') return counts.other.has_mail ? 'other' : 'all'
  return counts.addresses.some(({ id }) => id === requested) ? requested : 'all'
}

export function showAddressPicker(counts?: AddressCounts): boolean {
  return !!counts && (counts.addresses.length > 1 || counts.other.has_mail)
}

export function folderURL(segment: string, address: AddressFilter): string {
  const query = address === 'all' ? '' : `?address=${encodeURIComponent(address)}`
  return `/mail/${segment}${query}`
}

export function composeFrom(
  addresses: readonly MailboxAddressMatch[],
  selection: AddressFilter,
  matched?: readonly MailboxAddressMatch[],
): string {
  const sendable = addresses.filter((address) => address.can_send)
  const primary = sendable.find((address) => address.is_primary) ?? sendable[0]
  // An explicitly selected matching address is preferred for multi-recipient
  // replies. A selection unrelated to the source must not change its reply identity.
  const possible = matched ? sendable.filter(({ id }) => matched.some((item) => item.id === id)) : sendable
  const chosen = possible.find(({ id }) => id === selection) ??
    (matched && (possible.find((item) => item.is_primary) ?? possible[0])) ?? primary
  return chosen ? chosen.address : ''
}

export function draftsInAddressView(drafts: LocalDraft[], filter: AddressFilter, counts: AddressCounts | undefined, keyword: string): LocalDraft[] {
  if (filter === 'other') return []
  const match = counts?.addresses.find(({ id }) => id === filter)?.address?.toLowerCase()
  const query = keyword.trim().toLowerCase()
  return drafts.filter((draft) => (filter === 'all' || (!!match && draft.fromAddress?.toLowerCase() === match)) &&
    (!query || draft.subject.toLowerCase().includes(query) || draft.text.toLowerCase().includes(query)))
}
