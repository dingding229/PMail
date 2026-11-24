import { useCallback, useEffect, useRef } from 'react'
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from '@tanstack/react-query'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import {
  adminTrashApi,
  adminUpdateApi,
  authApi,
  groupApi,
  mailApi,
  signaturesApi,
  userAIApi,
  userApi,
  userTranslationApi,
  SYSTEM_GROUP_IDS,
  type AddressFilter,
  type DraftSaveRequest,
  type EmailDetail,
  type EmailListItem,
  type EmailListResponse,
} from '@/api'
import { getSearchTagForFolder, type FolderSelection } from '@/features/mail/types'

export const PAGE_SIZE = 50

export const queryKeys = {
  bootstrap: ['bootstrap'] as const,
  userMe: ['user', 'me'] as const,
  addresses: ['user', 'addresses'] as const,
  signatures: ['settings', 'signatures'] as const,
  sidebarGroups: ['groups', 'sidebar'] as const,
  sidebarGroupView: (address: AddressFilter) => ['groups', 'sidebar', 'filter', address] as const,
  addressCounts: ['groups', 'sidebar', 'address-counts'] as const,
  groupOptions: ['groups', 'list'] as const,
  emailListsRoot: ['emails', 'list'] as const,
  emailList: (tag: string, keyword: string, address: AddressFilter = 'all') => ['emails', 'list', tag, keyword, address] as const,
  emailDetail: (id: number) => ['emails', 'detail', id] as const,
  forward: ['settings', 'forward'] as const,
  rules: ['settings', 'rules'] as const,
  tokens: ['settings', 'tokens'] as const,
  spamSettings: ['settings', 'spam'] as const,
  spamRules: ['settings', 'spam', 'rules'] as const,
  queue: ['queue', 'list'] as const,
  usersRoot: ['admin', 'users'] as const,
  users: (page: number, pageSize: number) => ['admin', 'users', page, pageSize] as const,
  userAddresses: (userId: number) => ['admin', 'addresses', userId] as const,
  domains: ['admin', 'domains'] as const,
  adminInbound: ['admin', 'inbound'] as const,
  adminTelegram: ['admin', 'telegram'] as const,
  adminTrash: ['admin', 'trash'] as const,
  adminTranslation: ['admin', 'translation'] as const,
  adminUpdate: ['admin', 'update'] as const,
  userTelegram: ['settings', 'telegram'] as const,
  userTranslation: ['settings', 'translation'] as const,
  userAI: ['settings', 'ai'] as const,
  domainDns: (id: number) => ['admin', 'domains', 'dns', id] as const,
}

export function useBootstrapQuery() {
  return useQuery({
    queryKey: queryKeys.bootstrap,
    queryFn: ({ signal }) => authApi.bootstrap(signal),
    staleTime: 60_000,
    retry: 1,
  })
}

export function useUserMeQuery(enabled = true) {
  return useQuery({
    queryKey: queryKeys.userMe,
    queryFn: () => userApi.me(),
    enabled,
    staleTime: 5 * 60_000,
  })
}

export function useAddressesQuery(enabled = true) {
  return useQuery({
    queryKey: queryKeys.addresses,
    queryFn: () => userApi.listAddresses(),
    enabled,
    staleTime: 5 * 60_000,
  })
}

export function useSignaturesQuery(enabled = true) {
  return useQuery({
    queryKey: queryKeys.signatures,
    queryFn: () => signaturesApi.list(),
    enabled,
    staleTime: 2 * 60_000,
  })
}

export function useAdminUpdateQuery(enabled = true) {
  return useQuery({
    queryKey: queryKeys.adminUpdate,
    queryFn: () => adminUpdateApi.get(),
    enabled,
    staleTime: 5 * 60_000,
    retry: false,
  })
}

export function useAdminTrashQuery(enabled = true) {
  return useQuery({
    queryKey: queryKeys.adminTrash,
    queryFn: () => adminTrashApi.get(),
    enabled,
    staleTime: 5 * 60_000,
    retry: false,
  })
}

export function useUserTranslationPrefsQuery(enabled = true) {
  return useQuery({
    queryKey: queryKeys.userTranslation,
    queryFn: () => userTranslationApi.get(),
    enabled,
    staleTime: 2 * 60_000,
    retry: false,
  })
}

export function useUserAIQuery(enabled = true) {
  return useQuery({
    queryKey: queryKeys.userAI,
    queryFn: () => userAIApi.get(),
    enabled,
    staleTime: 2 * 60_000,
    retry: false,
  })
}

export function useAddressCountsQuery() {
  return useQuery({
    queryKey: queryKeys.addressCounts,
    queryFn: ({ signal }) => mailApi.addressCounts(signal),
    staleTime: 20_000,
    refetchInterval: 30_000,
  })
}

export function useSidebarGroupsQuery(enabled = true, address: AddressFilter = 'all') {
  return useQuery({
    queryKey: queryKeys.sidebarGroupView(address),
    queryFn: () => groupApi.sidebarTree(address),
    enabled,
    staleTime: 30_000,
  })
}

export function useGroupOptionsQuery(enabled = true) {
  return useQuery({
    queryKey: queryKeys.groupOptions,
    queryFn: () => groupApi.list(),
    enabled,
    staleTime: 30_000,
  })
}

export function useEmailInfiniteList(folder: FolderSelection, keyword: string, address: AddressFilter = 'all', enabled = true) {
  const tag = getSearchTagForFolder(folder)
  const trimmedKeyword = keyword.trim()

  return useInfiniteQuery({
    queryKey: queryKeys.emailList(tag, trimmedKeyword, address),
    enabled,
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      mailApi.list(
        {
          tag,
          keyword: trimmedKeyword,
          ...(address === 'all' ? {} : { address }),
          current_page: pageParam,
          page_size: PAGE_SIZE,
        },
        signal,
      ),
    getNextPageParam: (lastPage) => {
      if (!lastPage) return undefined
      if (lastPage.current_page < lastPage.total_page) {
        return lastPage.current_page + 1
      }
      return undefined
    },
    staleTime: 20_000,
  })
}

export function useEmailDetailQuery(id: number | null) {
  return useQuery({
    queryKey: queryKeys.emailDetail(id ?? 0),
    queryFn: ({ signal }) => mailApi.detail(id!, signal),
    enabled: id !== null && id > 0,
    staleTime: 2 * 60_000,
  })
}

export function usePrefetchEmailDetail() {
  const queryClient = useQueryClient()
  const timerRef = useRef<number | null>(null)

  const prefetch = useCallback(
    (id: number) => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current)
      }
      timerRef.current = window.setTimeout(() => {
        void queryClient.prefetchQuery({
          queryKey: queryKeys.emailDetail(id),
          queryFn: ({ signal }) => mailApi.detail(id, signal),
          staleTime: 2 * 60_000,
        })
      }, 65)
    },
    [queryClient],
  )

  const cancel = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  useEffect(() => cancel, [cancel])

  return { prefetch, cancel }
}

type EmailListCache = InfiniteData<EmailListResponse, number>
type DetailSnapshot = [readonly unknown[], EmailDetail | undefined]

function updateEmailListsInCache(
  queryClient: ReturnType<typeof useQueryClient>,
  updater: (items: EmailListItem[], queryKey: readonly unknown[]) => EmailListItem[],
) {
  const snapshots = queryClient.getQueriesData<EmailListCache>({
    queryKey: queryKeys.emailListsRoot,
  })

  for (const [key, old] of snapshots) {
    if (!old) continue
    queryClient.setQueryData<EmailListCache>(key, {
      ...old,
      pages: old.pages.map((page) => ({
        ...page,
        list: page.list ? updater(page.list, key) : page.list,
      })),
    })
  }

  return snapshots
}

function updateEmailDetailsInCache(
  queryClient: ReturnType<typeof useQueryClient>,
  ids: number[],
  updater: (detail: EmailDetail) => EmailDetail,
): DetailSnapshot[] {
  const snapshots: DetailSnapshot[] = []
  for (const id of ids) {
    const key = queryKeys.emailDetail(id)
    const prev = queryClient.getQueryData<EmailDetail>(key)
    snapshots.push([key, prev])
    if (prev) {
      queryClient.setQueryData<EmailDetail>(key, updater(prev))
    }
  }
  return snapshots
}

function restoreSnapshots(
  queryClient: ReturnType<typeof useQueryClient>,
  snapshots: [readonly unknown[], EmailListCache | undefined][],
  detailSnapshots?: DetailSnapshot[],
) {
  for (const [key, data] of snapshots) {
    queryClient.setQueryData(key, data)
  }
  if (detailSnapshots) {
    for (const [key, data] of detailSnapshots) {
      queryClient.setQueryData(key, data)
    }
  }
}

export function useStarEmailsMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: async ({ ids, starred }: { ids: number[]; starred: boolean; silent?: boolean }) => {
      return mailApi.star(ids, starred)
    },
    onMutate: async ({ ids, starred }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.emailListsRoot })
      const idSet = new Set(ids)
      const snapshots = updateEmailListsInCache(queryClient, (list, qKey) => {
        const tag = String(qKey[2] ?? '')
        const isStarredFolder = tag.includes('"starred":true')
        if (isStarredFolder && !starred) {
          return list.filter((item) => !idSet.has(item.id))
        }
        return list.map((item) =>
          idSet.has(item.id) ? { ...item, is_starred: starred } : item,
        )
      })
      const detailSnapshots = updateEmailDetailsInCache(queryClient, ids, (old) => ({
        ...old,
        is_starred: starred,
      }))
      return { snapshots, detailSnapshots }
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots, ctx.detailSnapshots)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
    onSuccess: (_res, vars, ctx) => {
      if (!vars.silent) {
        toast(vars.starred ? t('mail.toast.starred') : t('mail.toast.unstarred'), {
          action: {
            label: t('common.undo'),
            onClick: () => {
              if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots, ctx.detailSnapshots)
              void mailApi.star(vars.ids, !vars.starred).finally(() => {
                void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
                void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
              })
              toast.success(t('common.undone'))
            },
          },
        })
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    },
  })
}

export function useMarkReadMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: async ({ ids, isRead }: { ids: number[]; isRead: boolean; silent?: boolean }) => {
      return mailApi.markRead(ids, isRead)
    },
    onMutate: async ({ ids, isRead }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.emailListsRoot })
      const idSet = new Set(ids)
      const snapshots = updateEmailListsInCache(queryClient, (list) =>
        list.map((item) => (idSet.has(item.id) ? { ...item, is_read: isRead } : item)),
      )
      const detailSnapshots = updateEmailDetailsInCache(queryClient, ids, (old) => ({
        ...old,
        is_read: isRead ? 1 : 0,
      }))
      return { snapshots, detailSnapshots }
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots, ctx.detailSnapshots)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
    onSuccess: (_res, vars, ctx) => {
      if (!vars.silent) {
        toast(vars.isRead ? t('mail.toast.markedRead') : t('mail.toast.markedUnread'), {
          action: {
            label: t('common.undo'),
            onClick: () => {
              if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots, ctx.detailSnapshots)
              void mailApi.markRead(vars.ids, !vars.isRead).finally(() => {
                void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
                void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
              })
              toast.success(t('common.undone'))
            },
          },
        })
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    },
  })
}

export function useDeleteEmailsMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: async ({
      ids,
      forcedDel = false,
    }: {
      ids: number[]
      forcedDel?: boolean
      restoreGroupId?: number
    }) => {
      return mailApi.remove(ids, forcedDel)
    },
    onMutate: async ({ ids }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.emailListsRoot })
      const idSet = new Set(ids)
      const snapshots = updateEmailListsInCache(queryClient, (list) =>
        list.filter((item) => !idSet.has(item.id)),
      )
      return { snapshots }
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
    onSuccess: (_res, vars, ctx) => {
      if (vars.forcedDel) {
        toast.success(t('mail.toast.deletedForever', { count: vars.ids.length }))
      } else {
        const restoreTarget = vars.restoreGroupId ?? SYSTEM_GROUP_IDS.INBOX
        toast(t('mail.toast.deleted', { count: vars.ids.length }), {
          action: {
            label: t('common.undo'),
            onClick: async () => {
              if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots)
              try {
                await mailApi.move(vars.ids, restoreTarget)
                toast.success(t('common.undone'))
              } catch (e) {
                toast.error(e instanceof Error ? e.message : t('common.unknownError'))
              } finally {
                void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
                void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
              }
            },
          },
        })
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    },
  })
}

export function useMoveEmailsMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: async ({
      ids,
      groupId,
    }: {
      ids: number[]
      groupId: number
      targetLabel: string
      restoreGroupId?: number
    }) => {
      return mailApi.move(ids, groupId)
    },
    onMutate: async ({ ids }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.emailListsRoot })
      const idSet = new Set(ids)
      const snapshots = updateEmailListsInCache(queryClient, (list) =>
        list.filter((item) => !idSet.has(item.id)),
      )
      return { snapshots }
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
    onSuccess: (_res, vars, ctx) => {
      const restoreTarget = vars.restoreGroupId ?? SYSTEM_GROUP_IDS.INBOX
      toast(t('mail.toast.moved', { count: vars.ids.length, target: vars.targetLabel }), {
        action: {
          label: t('common.undo'),
          onClick: async () => {
            if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots)
            try {
              await mailApi.move(vars.ids, restoreTarget)
              toast.success(t('common.undone'))
            } catch (e) {
              toast.error(e instanceof Error ? e.message : t('common.unknownError'))
            } finally {
              void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
              void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
            }
          },
        },
      })
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    },
  })
}

export function useMarkSpamMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: async ({ ids, isSpam }: { ids: number[]; isSpam: boolean }) =>
      mailApi.markSpam(ids, isSpam),
    onMutate: async ({ ids }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.emailListsRoot })
      const idSet = new Set(ids)
      const snapshots = updateEmailListsInCache(queryClient, (list) =>
        list.filter((item) => !idSet.has(item.id)),
      )
      return { snapshots }
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
    onSuccess: (_res, vars) => {
      toast.success(
        vars.isSpam
          ? t('mail.toast.markedSpam', { count: vars.ids.length })
          : t('mail.toast.markedNotSpam', { count: vars.ids.length }),
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    },
  })
}

export function useSaveDraftMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (payload: DraftSaveRequest) => mailApi.saveDraft(payload),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    },
  })
}

export function useDeleteDraftMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => mailApi.deleteDraft(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    },
  })
}

export function useCreateGroupMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: (payload: { name: string; parent_id?: number }) => groupApi.add(payload),
    onSuccess: (_id, payload) => {
      toast.success(t('nav.groupCreated', { name: payload.name }))
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
      void queryClient.invalidateQueries({ queryKey: queryKeys.groupOptions })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })
}

export function useDeleteGroupMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: ({ id }: { id: number; name: string }) => groupApi.remove(id),
    onSuccess: (_ok, { name }) => {
      toast.success(t('nav.groupDeleted', { name }))
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
      void queryClient.invalidateQueries({ queryKey: queryKeys.groupOptions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })
}

export function useRestoreMailMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: (ids: number[]) => mailApi.restore(ids),
    onMutate: async (ids) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.emailListsRoot })
      const idSet = new Set(ids)
      const snapshots = updateEmailListsInCache(queryClient, (list) =>
        list.filter((item) => !idSet.has(item.id)),
      )
      return { snapshots }
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.snapshots) restoreSnapshots(queryClient, ctx.snapshots)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
    onSuccess: (res) => {
      toast.success(t('mail.trash.restoredToast', { count: res.restored }))
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
      void queryClient.invalidateQueries({ queryKey: queryKeys.addressCounts })
    },
  })
}

export function useEmptyTrashMutation() {
  const queryClient = useQueryClient()
  const { t } = useTranslation()

  return useMutation({
    mutationFn: () => mailApi.emptyTrash(),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    },
    onSuccess: (res) => {
      toast.success(t('mail.trash.emptiedToast', { count: res.deleted }))
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
      void queryClient.invalidateQueries({ queryKey: queryKeys.addressCounts })
    },
  })
}
