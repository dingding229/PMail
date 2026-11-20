<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, watch, computed } from 'vue'
import { NButton, NIcon, NEmpty, NTag, NSkeleton, useMessage } from 'naive-ui'
import {
  ArrowBackOutline,
  ArrowUndoOutline,
  ReturnUpForwardOutline,
  TrashOutline,
  DownloadOutline
} from '@vicons/ionicons5'
import { api, unwrap } from '@/api'
const props = defineProps({ mail: Object })
const emit = defineEmits(['back', 'deleted'])
const detail = ref(null),
  loading = ref(false),
  message = useMessage()
watch(
  () => props.mail,
  async (v) => {
    if (!v) {
      detail.value = null
      return
    }
    loading.value = true
    try {
      const res = await api.mail.detail(v.id)
      detail.value = unwrap(res)
    } finally {
      loading.value = false
    }
  },
  { immediate: true }
)
function html() {
  return detail.value?.html?.String || detail.value?.html || ''
}
const attachments = computed(() => {
  const value = detail.value?.attachments
  if (Array.isArray(value)) return value
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
  return []
})
function attachmentName(item, index) {
  return item.Filename || item.filename || item.name || `attachment-${index + 1}`
}
function attachmentSize(item) {
  const size = item.size || item.Size
  return size ? `${Math.ceil(size / 1024)} KB` : i18n.global.t('mail_sizeUnknown')
}
function compose(mode) {
  const d = detail.value
  const to = d?.from_address || ''
  location.hash = `/compose?mode=${mode}&email=${d?.id || ''}&to=${encodeURIComponent(to)}`
}
async function remove() {
  if (!detail.value) return
  await api.mail.remove(detail.value.id)
  message.success(i18n.global.t('mail_movedToTrash'))
  emit('deleted', detail.value.id)
}
</script>
<template>
  <section class="reader-pane">
    <template v-if="props.mail"
      ><div class="reader-toolbar">
        <n-button quaternary circle class="mobile-only" @click="emit('back')"
          ><n-icon><ArrowBackOutline /></n-icon
        ></n-button>
        <div class="reader-actions">
          <n-button
            secondary
            :aria-label="$t('legacy_ffc7850925')"
            @click="compose('reply')"
          >
            <n-icon><ArrowUndoOutline /></n-icon>{{ $t('legacy_ffc7850925') }}
          </n-button>
          <n-button
            secondary
            :aria-label="$t('mail_allReply')"
            @click="compose('replyAll')"
          >
            {{ $t('mail_allReply') }}
          </n-button>
          <n-button
            secondary
            :aria-label="$t('legacy_0d5a8ac36f')"
            @click="compose('forward')"
          >
            <n-icon><ReturnUpForwardOutline /></n-icon>{{ $t('legacy_0d5a8ac36f') }}
          </n-button
          ><n-button
            quaternary
            circle
            type="error"
            :aria-label="$t('legacy_3755f56f2f')"
            @click="remove"
            ><n-icon><TrashOutline /></n-icon
          ></n-button>
        </div>
      </div>
      <n-skeleton v-if="loading" text :repeat="8" />
      <article v-else-if="detail" class="reader-content">
        <div class="reader-heading">
          <n-tag size="small" type="info">{{
            detail.type === 1 ? $t('legacy_afb62932a7') : $t('legacy_1c8e464184')
          }}</n-tag>
          <h1>{{ detail.subject || $t('legacy_dd6026e30d') }}</h1>
          <div class="meta">
            <div class="avatar small">
              {{ (detail.from_name || detail.from_address || '?').slice(0, 1).toUpperCase() }}
            </div>
            <div>
              <strong>{{ detail.from_name || detail.from_address }}</strong><span
                >{{ detail.from_address }} · {{ detail.send_date }}</span
              >
            </div>
          </div>
        </div>
        <iframe
          v-if="html()"
          class="mail-html"
          sandbox="allow-popups allow-popups-to-escape-sandbox"
          :srcdoc="html()"
        />
        <pre v-else class="mail-text">{{ detail.text?.String || detail.text || '' }}</pre>
        <div v-if="attachments.length" class="attachments">
          <strong>{{ $t('legacy_99f6fe6c41') }}</strong>
          <div
            v-for="(item, index) in attachments"
            :key="`${attachmentName(item, index)}-${index}`"
            class="attachment-item"
          >
            <span>{{ attachmentName(item, index) }} · {{ attachmentSize(item) }}</span>
            <a
              :href="`/attachments/download/${detail.id}/${index}`"
              target="_blank"
              rel="noopener"
              >{{ $t('mail_download') }}</a
            >
          </div>
        </div>
      </article></template
    ><n-empty v-else :description="$t('legacy_25be5271f7')" class="reader-empty" />
  </section>
</template>
