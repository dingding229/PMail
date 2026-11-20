<script setup>
import { i18n } from '@/i18n/i18n'
import { ref } from 'vue'
import { NLayout, NLayoutSider, NLayoutContent, NMenu, NSelect } from 'naive-ui'
import ProfileSettings from '@/components/settings/ProfileSettings.vue'
import AddressSettings from '@/components/settings/AddressSettings.vue'
import ForwardSettings from '@/components/settings/ForwardSettings.vue'
import RuleSettings from '@/components/settings/RuleSettings.vue'
import ExternalSettings from '@/components/settings/ExternalSettings.vue'
import TokenSettings from '@/components/settings/TokenSettings.vue'
import QueueSettings from '@/components/settings/QueueSettings.vue'
import GroupSettings from '@/components/settings/GroupSettings.vue'
import PluginSettings from '@/components/settings/PluginSettings.vue'
import AppearanceSettings from '@/components/settings/AppearanceSettings.vue'
const active = ref('profile')
const menu = [
  { label: i18n.global.t('legacy_d562e344b9'), key: 'profile' },
  { label: i18n.global.t('legacy_684f53a787'), key: 'addresses' },
  { label: i18n.global.t('legacy_50cc88091b'), key: 'forwarding' },
  { label: i18n.global.t('legacy_ed904c685a'), key: 'rules' },
  { label: i18n.global.t('legacy_5bdba1e468'), key: 'external' },
  { label: 'API Token', key: 'tokens' },
  { label: i18n.global.t('legacy_f3ef3addc6'), key: 'queue' },
  { label: i18n.global.t('legacy_e900b55ce7'), key: 'groups' },
  { label: i18n.global.t('legacy_70a6a18cbf'), key: 'plugins' },
  { label: i18n.global.t('legacy_5c1a574bc2'), key: 'appearance' }
]
</script>
<template>
  <section class="settings-page">
    <n-select
      class="settings-mobile-tabs"
      v-model:value="active"
      :options="menu"
      :aria-label="$t('legacy_cb94344453')"
    />
    <n-layout has-sider class="settings-layout">
      <n-layout-sider width="220" bordered>
        <div class="settings-title">{{ $t('legacy_cb94344453') }}</div>
        <n-menu v-model:value="active" :options="menu" />
      </n-layout-sider>
      <n-layout-content class="settings-content">
        <ProfileSettings v-if="active === 'profile'" />
        <AddressSettings v-else-if="active === 'addresses'" />
        <ForwardSettings v-else-if="active === 'forwarding'" />
        <RuleSettings v-else-if="active === 'rules'" />
        <ExternalSettings v-else-if="active === 'external'" />
        <TokenSettings v-else-if="active === 'tokens'" />
        <QueueSettings v-else-if="active === 'queue'" />
        <GroupSettings v-else-if="active === 'groups'" />
        <PluginSettings v-else-if="active === 'plugins'" />
        <AppearanceSettings v-else />
      </n-layout-content>
    </n-layout>
  </section>
</template>
