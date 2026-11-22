import { useNavigate, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import {
  AtSign,
  Globe,
  SendToBack,
  Settings,
  ShieldAlert,
  ShieldCheck,
  Users,
} from 'lucide-react'
import { useUserMeQuery } from '@/hooks/useMailQueries'
import { prefetchSettingsPage } from '@/lib/prefetch'
import { Button } from '@/components/ui/button'
import { SectionPanelShell, type PanelTabItem } from '@/components/common/SectionPanelShell'
import { QueueSection } from '@/features/settings/sections/QueueSection'
import { DomainsSection } from './DomainsSection'
import { UsersSection } from './UsersSection'
import { AdminAddressesSection } from './AdminAddressesSection'
import { InboundSecuritySection } from './InboundSecuritySection'

type AdminTabKey = 'domains' | 'users' | 'addresses' | 'inbound' | 'queue'

export function AdminPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { tab } = useParams<{ tab?: string }>()
  const { data: user } = useUserMeQuery(true)

  const tabs: PanelTabItem<AdminTabKey>[] = [
    { key: 'domains', label: t('admin.tabs.domains'), icon: Globe },
    { key: 'users', label: t('admin.tabs.users'), icon: Users },
    { key: 'addresses', label: t('admin.tabs.addresses'), icon: AtSign },
    { key: 'inbound', label: t('admin.tabs.inbound'), icon: ShieldCheck },
    { key: 'queue', label: t('admin.tabs.queue'), icon: SendToBack },
  ]

  const activeTab: AdminTabKey = tabs.some((x) => x.key === tab)
    ? (tab as AdminTabKey)
    : 'domains'

  const forbiddenView =
    user && user.is_admin !== 1 ? (
      <main className="mx-auto max-w-xl px-4 py-16 text-center">
        <div className="flex flex-col items-center rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-10">
          <ShieldAlert className="h-9 w-9 text-[var(--danger)]" />
          <p className="mt-3 text-[14.5px] font-medium">{t('admin.noPermission')}</p>
          <Button
            variant="secondary"
            size="sm"
            className="mt-4"
            onClick={() => navigate('/mail/inbox')}
          >
            {t('admin.backToMail')}
          </Button>
        </div>
      </main>
    ) : undefined

  return (
    <SectionPanelShell
      title={t('admin.title')}
      backLabel={t('admin.backToMail')}
      onBack={() => navigate('/mail/inbox')}
      headerActions={
        <Button
          variant="outline"
          size="xs"
          onMouseEnter={prefetchSettingsPage}
          onFocus={prefetchSettingsPage}
          onClick={() => navigate('/settings')}
        >
          <Settings className="h-3.5 w-3.5" />
          <span className="hidden sm:inline">{t('nav.settings')}</span>
        </Button>
      }
      tabs={tabs}
      activeTab={activeTab}
      onSelectTab={(next) => navigate(`/admin/${next}`)}
      guardContent={forbiddenView}
    >
      {activeTab === 'domains' && <DomainsSection />}
      {activeTab === 'users' && <UsersSection />}
      {activeTab === 'addresses' && <AdminAddressesSection />}
      {activeTab === 'inbound' && <InboundSecuritySection />}
      {activeTab === 'queue' && <QueueSection scope="global" />}
    </SectionPanelShell>
  )
}

export default AdminPage
