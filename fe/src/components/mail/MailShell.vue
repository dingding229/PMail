<script setup>
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NButton,
  NIcon,
  NDrawer,
  NDrawerContent,
  useDialog
} from 'naive-ui'
import { MenuOutline, MoonOutline, SunnyOutline, SettingsOutline } from '@vicons/ionicons5'
import MailSidebar from './MailSidebar.vue'
import MailList from './MailList.vue'
import MailReader from './MailReader.vue'
import MobileBottomNav from './MobileBottomNav.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { useMailStore } from '@/stores/mail'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const ui = useUiStore()
const mail = useMailStore()
const dialog = useDialog()
const selected = ref(null)
const mobile = ref(false)
const listWidth = ref(Number(localStorage.getItem('pmail-list-width') || 520))
const dragging = ref(null)
const dragStart = ref(0)
const dragOrigin = ref(0)
const isMail = computed(() => route.name === 'mail')
const title = computed(() =>
  route.name === 'compose' ? t('compose_title') : selected.value?.subject || t('common_inbox')
)

function select(value) {
  selected.value = value
}

async function handleDeleted() {
  selected.value = null
  if (mail.list.length === 1 && mail.page > 1) mail.page -= 1
  await mail.load()
}

function logout() {
  dialog.warning({
    title: t('shell_logoutTitle'),
    content: t('shell_logoutConfirm'),
    positiveText: t('common_logout'),
    negativeText: t('common_cancel'),
    onPositiveClick: () => auth.logout()
  })
}

function startResize(kind, event) {
  if (mobile.value) return
  dragging.value = kind
  dragStart.value = event.clientX
  dragOrigin.value = kind === 'sidebar' ? ui.sidebarWidth : listWidth.value
  window.addEventListener('pointermove', onResize)
  window.addEventListener('pointerup', stopResize, { once: true })
  event.currentTarget?.setPointerCapture?.(event.pointerId)
}

function onResize(event) {
  if (!dragging.value) return
  const delta = event.clientX - dragStart.value
  if (dragging.value === 'sidebar') {
    ui.setSidebarWidth(Math.min(360, Math.max(180, dragOrigin.value + delta)))
  } else {
    listWidth.value = Math.min(900, Math.max(320, dragOrigin.value + delta))
    localStorage.setItem('pmail-list-width', String(listWidth.value))
  }
}

function stopResize() {
  dragging.value = null
  window.removeEventListener('pointermove', onResize)
}

function updateMobile(event) {
  mobile.value = event.matches
}

onMounted(() => {
  mail.load()
  mail.loadGroups()
  const query = matchMedia('(max-width: 767px)')
  mobile.value = query.matches
  query.addEventListener('change', updateMobile)
})
onBeforeUnmount(() => {
  stopResize()
  matchMedia('(max-width: 767px)').removeEventListener('change', updateMobile)
})
</script>

<template>
  <n-layout class="mail-shell">
    <n-layout-header class="topbar">
      <div class="brand">
        <n-button text class="mobile-menu" :aria-label="t('shell_menu')" @click="ui.drawer = true">
          <n-icon><MenuOutline /></n-icon>
        </n-button>
        <span class="brand-mark">P</span><strong>PMail</strong
        ><span class="crumb">{{ title }}</span>
      </div>
      <div class="top-actions">
        <span class="account-name">{{
          auth.user?.display_name || auth.user?.name || auth.user?.account
        }}</span>
        <n-button quaternary circle :aria-label="t('shell_toggleTheme')" @click="ui.toggleDark()">
          <n-icon><MoonOutline v-if="!ui.dark" /><SunnyOutline v-else /></n-icon>
        </n-button>
        <n-button
          quaternary
          circle
          :aria-label="t('common_settings')"
          @click="router.push('/settings')"
        >
          <n-icon><SettingsOutline /></n-icon>
        </n-button>
        <n-button quaternary @click="logout">{{ t('shell_logout') }}</n-button>
      </div>
    </n-layout-header>
    <n-layout has-sider class="body-layout">
      <n-layout-sider v-if="!mobile" bordered :width="ui.sidebarWidth" :native-scrollbar="false">
        <MailSidebar />
        <div
          class="resize-handle sidebar-handle"
          role="separator"
          :aria-label="t('shell_resizeSidebar')"
          @pointerdown="startResize('sidebar', $event)"
        />
      </n-layout-sider>
      <n-drawer v-model:show="ui.drawer" placement="left" :width="280">
        <n-drawer-content body-content-style="padding:0"
          ><MailSidebar @close="ui.drawer = false"
        /></n-drawer-content>
      </n-drawer>
      <n-layout-content class="content-wrap">
        <router-view v-if="!isMail" />
        <div
          v-else
          class="mail-workspace"
          :class="{ mobile }"
          :style="{
            '--mail-list-width': `${listWidth}px`,
            '--mail-reader-width': `${ui.readerWidth}px`
          }"
        >
          <MailList @select="select" :selected-id="selected?.id" />
          <div
            class="resize-handle list-handle"
            role="separator"
            :aria-label="t('shell_resizeList')"
            @pointerdown="startResize('list', $event)"
          />
          <MailReader v-if="!mobile" :mail="selected" @deleted="handleDeleted" />
          <n-drawer
            v-if="mobile && selected"
            :show="true"
            placement="right"
            width="100%"
            class="mobile-reader-drawer"
            @update:show="(value) => !value && (selected = null)"
          >
            <n-drawer-content
              class="mobile-reader-content"
              body-content-style="padding:0; height:100%;"
            >
              <MailReader v-if="selected" :mail="selected" @back="selected = null" @deleted="handleDeleted" />
            </n-drawer-content>
          </n-drawer>
        </div>
      </n-layout-content>
    </n-layout>
    <MobileBottomNav v-if="mobile" />
  </n-layout>
</template>
