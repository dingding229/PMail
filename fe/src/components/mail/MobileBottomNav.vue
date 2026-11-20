<script setup>
import { NButton, NIcon } from 'naive-ui'
import { MailOutline, FolderOpenOutline, CreateOutline, SettingsOutline } from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
const router = useRouter()
</script>
<template>
  <nav class="bottom-nav">
    <n-button text @click="router.push('/mail')"
      ><n-icon><MailOutline /></n-icon><span>{{ $t('legacy_cd83e6e100') }}</span></n-button
    ><n-button text @click="router.push('/mail?folder=groups')"
      ><n-icon><FolderOpenOutline /></n-icon><span>{{ $t('legacy_46ecac2910') }}</span></n-button
    ><n-button text type="primary" @click="router.push('/compose')"
      ><n-icon><CreateOutline /></n-icon><span>{{ $t('legacy_dff028bb70') }}</span></n-button
    ><n-button text @click="router.push('/settings')"
      ><n-icon><SettingsOutline /></n-icon><span>{{ $t('legacy_7debf9cb03') }}</span></n-button
    >
  </nav>
</template>
