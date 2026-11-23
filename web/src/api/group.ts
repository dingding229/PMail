import { apiPost } from './client'
import type { AddressFilter, GroupOption, GroupTreeItem } from './types'

export const groupApi = {
  /** Default + custom group tree for navigation sidebar */
  sidebarTree: (address: AddressFilter = 'all') => apiPost<GroupTreeItem[]>('/api/group', address === 'all' ? {} : { address }),

  /** Flat list of movable destinations (INBOX, Junk, Deleted + custom groups) */
  list: () => apiPost<GroupOption[]>('/api/group/list', {}),

  add: (payload: { name: string; parent_id?: number }) =>
    apiPost<number>('/api/group/add', { name: payload.name, parent_id: payload.parent_id ?? 0 }),

  remove: (id: number) => apiPost<boolean>('/api/group/del', { id }),
}
