import { useNavigate, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import {
  AtSign,
  Filter,
  Folder,
  Forward,
  KeyRound,
  Palette,
  PenLine,
  Send,
  SendToBack,
  Shield,
  ShieldBan,
  UserCog,
} from 'lucide-react'
import { useUserMeQuery } from '@/hooks/useMailQueries'
import { prefetchAdminPage } from '@/lib/prefetch'
import { Button } from '@/components/ui/button'
import { SectionPanelShell, type PanelTabItem } from '@/components/common/SectionPanelShell'
import { ProfileSecuritySection } from './sections/ProfileSecuritySection'
import { AppearanceSection } from './sections/AppearanceSection'
import { AddressesSection } from './sections/AddressesSection'
import { SignaturesSection } from './sections/SignaturesSection'
import { ForwardSection } from './sections/ForwardSection'
import { RulesSection } from './sections/RulesSection'
import { SpamSection } from './sections/SpamSection'
import { TelegramSection } from './sections/TelegramSection'
import { GroupsSection } from './sections/GroupsSection'
import { TokensSection } from './sections/TokensSection'
import { QueueSection } from './sections/QueueSection'

type SettingsTabKey =
  | 'profile'
  | 'appearance'
  | 'addresses'
  | 'signatures'
  | 'forward'
  | 'rules'
  | 'spam'
  | 'telegram'
  | 'groups'
  | 'tokens'
  | 'queue'

export function SettingsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { tab } = useParams<{ tab?: string }>()
  const { data: me } = useUserMeQuery(true)

  const tabs: PanelTabItem<SettingsTabKey>[] = [
    { key: 'profile', label: t('settings.tabs.profile'), icon: UserCog },
    { key: 'appearance', label: t('settings.tabs.appearance'), icon: Palette },
    { key: 'addresses', label: t('settings.tabs.addresses'), icon: AtSign },
    { key: 'signatures', label: t('settings.tabs.signatures'), icon: PenLine },
    { key: 'forward', label: t('settings.tabs.forward'), icon: Forward },
    { key: 'rules', label: t('settings.tabs.rules'), icon: Filter },
    { key: 'spam', label: t('settings.tabs.spam'), icon: ShieldBan },
    { key: 'telegram', label: t('settings.tabs.telegram'), icon: Send },
    { key: 'groups', label: t('settings.tabs.groups'), icon: Folder },
    { key: 'tokens', label: t('settings.tabs.tokens'), icon: KeyRound },
    { key: 'queue', label: t('settings.tabs.queue'), icon: SendToBack },
  ]

  const activeTab: SettingsTabKey = tabs.some((x) => x.key === tab)
    ? (tab as SettingsTabKey)
    : 'profile'

  return (
    <SectionPanelShell
      title={t('settings.title')}
      backLabel={t('settings.backToMail')}
      onBack={() => navigate('/mail/inbox')}
      headerActions={
        me?.is_admin === 1 ? (
          <Button
            variant="outline"
            size="xs"
            onMouseEnter={prefetchAdminPage}
            onFocus={prefetchAdminPage}
            onClick={() => navigate('/admin')}
          >
            <Shield className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">{t('nav.admin')}</span>
          </Button>
        ) : undefined
      }
      tabs={tabs}
      activeTab={activeTab}
      onSelectTab={(next) => navigate(`/settings/${next}`)}
    >
      {activeTab === 'profile' && <ProfileSecuritySection />}
      {activeTab === 'appearance' && <AppearanceSection />}
      {activeTab === 'addresses' && <AddressesSection />}
      {activeTab === 'signatures' && <SignaturesSection />}
      {activeTab === 'forward' && <ForwardSection />}
      {activeTab === 'rules' && <RulesSection />}
      {activeTab === 'spam' && <SpamSection />}
      {activeTab === 'telegram' && <TelegramSection />}
      {activeTab === 'groups' && <GroupsSection />}
      {activeTab === 'tokens' && <TokensSection />}
      {activeTab === 'queue' && <QueueSection scope="mine" />}
    </SectionPanelShell>
  )
}

export default SettingsPage
