<script setup>
import { i18n } from '@/i18n/i18n'
import { computed } from 'vue'
import { NButton, NIcon, NDivider, NTag } from 'naive-ui'
import {
  MailOutline,
  SendOutline,
  DocumentTextOutline,
  TrashOutline,
  WarningOutline,
  FolderOpenOutline,
  AddOutline,
  SettingsOutline,
  ShieldCheckmarkOutline
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useMailStore } from '@/stores/mail'
import { useAuthStore } from '@/stores/auth'
const emit = defineEmits(['close'])
const router = useRouter(),
  mail = useMailStore(),
  auth = useAuthStore()
const folders = [
  ['{"type":0,"status":-1}', i18n.global.t('legacy_cd83e6e100'), MailOutline],
  ['{"type":1,"status":1}', i18n.global.t('legacy_afb62932a7'), SendOutline],
  ['{"type":-1,"status":4}', i18n.global.t('legacy_0f436818c0'), DocumentTextOutline],
  ['{"type":-1,"status":5}', i18n.global.t('legacy_a3093448fd'), WarningOutline],
  ['{"type":-1,"status":3}', i18n.global.t('legacy_fb5fe1e266'), TrashOutline]
]
function openCompose() {
  router.push('/compose')
  emit('close')
}
function openSettings() {
  router.push('/settings')
  emit('close')
}
function openAdmin() {
  router.push('/admin')
  emit('close')
}
function go(tag) {
  mail.setFolder(tag)
  emit('close')
}
</script>
<template>
  <aside class="sidebar">
    <div class="profile">
      <div class="avatar">
        {{ (auth.user?.name || auth.user?.account || 'P').slice(0, 1).toUpperCase() }}
      </div>
      <div>
        <strong>{{
          auth.user?.display_name || auth.user?.name || auth.user?.account
        }}</strong><small>{{ auth.user?.account }}</small>
      </div>
    </div>
    <n-button type="primary" block size="large" class="compose-action" @click="openCompose"
      ><n-icon><AddOutline /></n-icon>{{ $t('legacy_dff028bb70') }}</n-button
    >
    <nav class="folder-nav">
      <button
        v-for="f in folders"
        :key="f[0]"
        :class="{ active: mail.tag === f[0] }"
        @click="go(f[0])"
      >
        <n-icon><component :is="f[2]" /></n-icon><span>{{ f[1] }}</span>
      </button>
    </nav>
    <n-divider />
    <div class="section-label">{{ $t('legacy_97d8a6c05b') }}</div>
    <button
      class="side-link"
      v-for="g in mail.groups"
      :key="g.id"
      @click="go(JSON.stringify({ type: -1, status: -1, group_id: g.id }))"
    >
      <n-icon><FolderOpenOutline /></n-icon>{{ g.name }}
    </button>
    <div class="sidebar-spacer" />
    <button class="side-link" @click="openSettings">
      <n-icon><SettingsOutline /></n-icon>{{ $t('legacy_7debf9cb03') }}</button
    ><button v-if="auth.isAdmin" class="side-link" @click="openAdmin">
      <n-icon><ShieldCheckmarkOutline /></n-icon>{{ $t('common_admin') }}
    </button>
  </aside>
</template>
