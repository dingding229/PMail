<script setup>
import { computed, onMounted, watchEffect } from 'vue'
import { NConfigProvider, NMessageProvider, NDialogProvider, darkTheme } from 'naive-ui'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
const route = useRoute(),
  auth = useAuthStore(),
  ui = useUiStore()
const shell = computed(() => !['login', 'setup'].includes(route.name))
watchEffect(() => {
  const root = document.documentElement
  root.classList.toggle('dark', ui.dark)
  root.style.colorScheme = ui.dark ? 'dark' : 'light'
})
onMounted(() => auth.load())
</script>
<template>
  <n-config-provider
    :theme="ui.dark ? darkTheme : null"
    :theme-overrides="{
      common: { primaryColor: '#5b5ce2', primaryColorHover: '#7475ef', borderRadius: '12px' }
    }"
  >
    <n-message-provider
      ><n-dialog-provider>
        <div class="app-root" :class="{ dark: ui.dark }">
          <router-view v-if="!shell" /><MailShell v-else />
        </div> </n-dialog-provider
    ></n-message-provider>
  </n-config-provider>
</template>
<script>
import MailShell from './components/mail/MailShell.vue'
</script>
