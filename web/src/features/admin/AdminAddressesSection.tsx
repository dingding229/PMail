import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { userApi } from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AddressesSection } from '@/features/settings/sections/AddressesSection'

export function AdminAddressesSection() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: queryKeys.users(1, 100),
    queryFn: () => userApi.list({ current_page: 1, page_size: 100 }),
  })

  const users = useMemo(() => data?.list ?? [], [data?.list])
  const [selectedUserId, setSelectedUserId] = useState<number>(0)

  useEffect(() => {
    if (!selectedUserId && users[0]) {
      setSelectedUserId(users[0].id)
    }
  }, [selectedUserId, users])

  return (
    <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)] space-y-5">
      <div>
        <h2 className="text-[15px] font-semibold text-[var(--ink-primary)]">
          {t('admin.addresses.title')}
        </h2>
        <p className="mt-0.5 text-[12.5px] text-[var(--ink-secondary)]">
          {t('admin.addresses.desc')}
        </p>
      </div>

      {isLoading ? (
        <Skeleton className="h-32 w-full rounded-[var(--radius-md)]" />
      ) : (
        <>
          <div className="max-w-xs space-y-1.5">
            <Label>{t('admin.addresses.selectUser')}</Label>
            <Select
              value={selectedUserId ? String(selectedUserId) : ''}
              onValueChange={(v) => setSelectedUserId(Number(v))}
            >
              <SelectTrigger className="h-9">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {users.map((u) => (
                  <SelectItem key={u.id} value={String(u.id)}>
                    {u.display_name || u.name} ({u.account})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {selectedUserId > 0 && <AddressesSection userId={selectedUserId} embedded />}
        </>
      )}
    </section>
  )
}
