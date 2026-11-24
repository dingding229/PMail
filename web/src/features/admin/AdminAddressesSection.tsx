import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AtSign } from 'lucide-react'
import { userApi } from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  FormRow,
  SectionCard,
  SectionErrorState,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'
import { AddressesSection } from '@/features/settings/sections/AddressesSection'

export function AdminAddressesSection() {
  const { t } = useTranslation()
  const { data, isLoading, error, refetch } = useQuery({
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
    <SectionCard>
      <SectionHeader
        icon={AtSign}
        title={t('admin.addresses.title')}
        description={t('admin.addresses.desc')}
      />

      {error ? (
        <SectionErrorState
          message={error instanceof Error ? error.message : t('common.unknownError')}
          retryLabel={t('app.retry')}
          onRetry={() => void refetch()}
        />
      ) : isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-20" />
      ) : (
        <>
          <FormRow label={t('admin.addresses.selectUser')} className="max-w-sm">
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
          </FormRow>

          {selectedUserId > 0 && <AddressesSection userId={selectedUserId} embedded />}
        </>
      )}
    </SectionCard>
  )
}
