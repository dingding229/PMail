<script setup>
import { i18n } from '@/i18n/i18n'
import { ref } from 'vue'
import { NLayout, NLayoutSider, NLayoutContent, NMenu, NSelect } from 'naive-ui'
import UserAdmin from '@/components/admin/UserAdmin.vue'
import DomainAdmin from '@/components/admin/DomainAdmin.vue'
import AddressAdmin from '@/components/admin/AddressAdmin.vue'
import QueueAdmin from '@/components/admin/QueueAdmin.vue'
const active = ref('users')
const menu = [
  { label: i18n.global.t('legacy_baf84751a2'), key: 'users' },
  { label: i18n.global.t('legacy_96de09f813'), key: 'domains' },
  { label: i18n.global.t('legacy_045c2d9263'), key: 'addresses' },
  { label: i18n.global.t('legacy_43f400a633'), key: 'queue' }
]
</script>
<template>
  <section class="admin-page">
    <n-select
      class="settings-mobile-tabs"
      v-model:value="active"
      :options="menu"
      :aria-label="$t('legacy_982a5fe0e8')"
    />
    <n-layout has-sider class="settings-layout">
      <n-layout-sider width="220" bordered>
        <div class="settings-title">{{ $t('legacy_982a5fe0e8') }}</div>
        <n-menu v-model:value="active" :options="menu" />
      </n-layout-sider>
      <n-layout-content class="settings-content">
        <UserAdmin v-if="active === 'users'" />
        <DomainAdmin v-else-if="active === 'domains'" />
        <AddressAdmin v-else-if="active === 'addresses'" />
        <QueueAdmin v-else />
      </n-layout-content>
    </n-layout>
  </section>
</template>
