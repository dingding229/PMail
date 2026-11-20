<script setup>
import { ref, computed } from 'vue'
import {
  NInput,
  NButton,
  NIcon,
  NCheckbox,
  NSpin,
  NPagination,
  NEmpty,
  NTag,
  useMessage,
  useDialog
} from 'naive-ui'
import { SearchOutline, RefreshOutline, TrashOutline, EyeOutline } from '@vicons/ionicons5'
import { useMailStore } from '@/stores/mail'
import { api } from '@/api'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const props = defineProps({ selectedId: [Number, String] })
const emit = defineEmits(['select'])
const mail = useMailStore()
const message = useMessage()
const dialog = useDialog()
const keyword = ref(mail.keyword)
const selected = ref([])
const swipeRow = ref(null)
const touchStartX = ref(0)
const touchStartY = ref(0)
const pullStartY = ref(0)
const pulling = ref(false)
const totalPages = computed(() => Math.max(1, mail.total || 1))
const moveTarget = ref(null)
const moveOptions = computed(() => [
  { label: t('mail_inbox'), value: 2000000000 },
  { label: t('mail_sent'), value: 2000000001 },
  { label: t('mail_drafts'), value: 2000000002 },
  { label: t('mail_deleted'), value: 2000000003 },
  { label: t('mail_junk'), value: 2000000004 },
  ...mail.groups.map((group) => ({ label: group.name, value: group.id }))
])

async function search() {
  mail.keyword = keyword.value
  await mail.load()
}
async function moveSelected() {
  if (!moveTarget.value || !selected.value.length) {
    message.warning(t('mail_moveRequired'))
    return
  }
  const res = await api.mail.move(
    selected.value.map((row) => row.id),
    moveTarget.value
  )
  if (res.errorNo) message.error(res.errorMsg || t('common_httpError'))
  else {
    message.success(t('mail_moveSuccess'))
    selected.value = []
    moveTarget.value = null
    await mail.load()
  }
}
async function refresh() {
  await mail.load()
  message.success(t('common_refreshed'))
}
async function markRows(rows = selected.value) {
  for (const row of rows) await api.mail.read(row.id)
  selected.value = []
  swipeRow.value = null
  await mail.load()
}
function removeRows(rows = selected.value) {
  if (!rows.length) return
  dialog.warning({
    title: t('mail_deleteTitle'),
    content: t('mail_deleteConfirm'),
    positiveText: t('common_delete'),
    negativeText: t('common_cancel'),
    onPositiveClick: async () => {
      await api.mail.remove(rows.map((row) => row.id))
      selected.value = []
      swipeRow.value = null
      await mail.load()
    }
  })
}
function toggle(row, value) {
  selected.value = value
    ? [...selected.value, row]
    : selected.value.filter((item) => item.id !== row.id)
}
function rowClick(row) {
  if (swipeRow.value === row.id) {
    swipeRow.value = null
    return
  }
  emit('select', row)
  api.mail.read(row.id)
  row.is_read = 1
}
function startRowTouch(row, event) {
  touchStartX.value = event.touches[0].clientX
  touchStartY.value = event.touches[0].clientY
  swipeRow.value = row.id
}
function moveRowTouch(event) {
  const deltaX = event.touches[0].clientX - touchStartX.value
  const deltaY = event.touches[0].clientY - touchStartY.value
  if (Math.abs(deltaX) > Math.abs(deltaY) && deltaX < -20)
    event.currentTarget.style.transform = `translateX(${Math.max(deltaX, -150)}px)`
}
function endRowTouch(event, row) {
  const deltaX = event.changedTouches[0].clientX - touchStartX.value
  event.currentTarget.style.transform = ''
  swipeRow.value = deltaX < -40 ? row.id : null
}
function startPull(event) {
  if (event.target.closest('.rows')?.scrollTop > 0) return
  pullStartY.value = event.touches[0].clientY
  pulling.value = true
}
async function endPull(event) {
  if (!pulling.value) return
  pulling.value = false
  if (event.changedTouches[0].clientY - pullStartY.value > 70) await refresh()
}
function date(value) {
  return value
    ? new Date(value).toLocaleString([], {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
      })
    : ''
}
</script>

<template>
  <section class="mail-list-pane" @touchstart.passive="startPull" @touchend.passive="endPull">
    <div class="list-toolbar">
      <n-input
        v-model:value="keyword"
        round
        :placeholder="t('common_searchMail')"
        @keyup.enter="search"
      >
        <template #prefix
          ><n-icon><SearchOutline /></n-icon
        ></template>
      </n-input>
      <n-button quaternary circle :aria-label="t('common_search')" @click="search"
        ><n-icon><SearchOutline /></n-icon
      ></n-button>
      <n-button quaternary circle :aria-label="t('common_refresh')" @click="refresh"
        ><n-icon><RefreshOutline /></n-icon
      ></n-button>
    </div>
    <div class="bulkbar">
      <n-checkbox
        :checked="selected.length === mail.list.length && mail.list.length > 0"
        @update:checked="(value) => (selected = value ? [...mail.list] : [])"
      >
        {{ t('mail_selectAll') }}
      </n-checkbox>
      <n-button text size="small" :disabled="!selected.length" @click="markRows()"
        ><n-icon><EyeOutline /></n-icon>{{ t('common_markRead') }}</n-button
      >
      <n-button text size="small" type="error" :disabled="!selected.length" @click="removeRows()"
        ><n-icon><TrashOutline /></n-icon>{{ t('common_delete') }}</n-button
      >
      <n-select
        v-model:value="moveTarget"
        size="small"
        clearable
        :options="moveOptions"
        :placeholder="t('mail_moveTo')"
        :disabled="!selected.length"
        @update:value="moveSelected"
      />
    </div>
    <n-spin :show="mail.loading">
      <div v-if="mail.list.length" class="rows">
        <article
          v-for="row in mail.list"
          :key="row.id"
          class="mail-row"
          :class="{ active: props.selectedId === row.id, unread: !row.is_read }"
          @click="rowClick(row)"
          @touchstart.stop="startRowTouch(row, $event)"
          @touchmove.stop="moveRowTouch"
          @touchend.stop="endRowTouch($event, row)"
        >
          <n-checkbox
            :checked="selected.some((item) => item.id === row.id)"
            @update:checked="(value) => toggle(row, value)"
            @click.stop
          />
          <div class="row-main">
            <div class="row-top">
              <strong>{{
                row.sender?.Name || row.sender?.EmailAddress || row.from_address
              }}</strong><time>{{ date(row.datetime) }}</time>
            </div>
            <div class="row-subject" @click.stop="rowClick(row)">
              {{ row.title || t('mail_noSubject')
              }}<n-tag v-if="row.error" type="error" size="small">{{ t('common_failed') }}</n-tag>
            </div>
            <p>{{ row.desc }}</p>
          </div>
          <div v-if="swipeRow === row.id" class="swipe-actions" @click.stop>
            <n-button size="small" type="warning" @click="markRows([row])">{{
              t('common_markRead')
            }}</n-button>
            <n-button size="small" type="error" @click="removeRows([row])">{{
              t('common_delete')
            }}</n-button>
          </div>
        </article>
      </div>
      <n-empty v-else :description="t('common_empty')" class="empty-list" />
    </n-spin>
    <div class="list-footer">
      <span>{{ mail.total ? t('mail_page', { page: mail.page }) : '' }}</span>
      <n-pagination
        v-model:page="mail.page"
        :page-count="totalPages"
        size="small"
        @update:page="mail.load"
      />
    </div>
  </section>
</template>
