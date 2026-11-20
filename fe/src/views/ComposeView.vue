<script setup>
import { ref, onBeforeUnmount, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NCard,
  NButton,
  NInput,
  NSelect,
  NForm,
  NFormItem,
  NIcon,
  NTag,
  NSwitch,
  NAlert,
  NSpace,
  useMessage
} from 'naive-ui'
import {
  ArrowBackOutline,
  AttachOutline,
  SendOutline,
  SaveOutline,
  CloseOutline,
  LinkOutline
} from '@vicons/ionicons5'
import { useAuthStore } from '@/stores/auth'
import { api, unwrap } from '@/api'
import { useI18n } from 'vue-i18n'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Image from '@tiptap/extension-image'
import Underline from '@tiptap/extension-underline'
import TextAlign from '@tiptap/extension-text-align'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const message = useMessage()
const addresses = ref([])
const to = ref([])
const cc = ref([])
const bcc = ref([])
const subject = ref('')
const from = ref('')
const attachments = ref([])
const includeAttachments = ref(true)
const sending = ref(false)
const loadingOriginal = ref(false)
const error = ref('')
const deliveryResults = ref([])
const forwardEmailId = ref(0)
const mode = computed(() => String(route.query.mode || 'compose'))
const isForward = computed(() => mode.value === 'forward' && forwardEmailId.value > 0)
const draftKey = 'pmail-compose-draft'

const editor = useEditor({
  extensions: [
    StarterKit,
    Link.configure({ openOnClick: false }),
    Image,
    Underline,
    TextAlign.configure({ types: ['heading', 'paragraph'] })
  ],
  content: '<p></p>'
})

const sendableAddresses = computed(() => addresses.value.filter((item) => item.can_send))
const addressOptions = computed(() =>
  sendableAddresses.value.map((item) => ({ label: item.address, value: item.address }))
)
const recipientOptions = computed(() =>
  addresses.value.map((item) => ({ label: item.address, value: item.address }))
)

async function loadAddresses() {
  const res = await api.addresses.list()
  if (!res.errorNo) {
    addresses.value = unwrap(res, []) || []
    from.value =
      sendableAddresses.value.find((item) => item.is_primary)?.address ||
      sendableAddresses.value[0]?.address ||
      ''
  }
}

function quoteText(value) {
  return String(value || '')
    .split('\n')
    .map((line) => `> ${line}`)
    .join('\n')
}

function parseAddressList(value) {
  if (Array.isArray(value)) return value
  if (typeof value !== 'string' || !value.trim()) return []
  try {
    const parsed = JSON.parse(value)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function addressValue(item) {
  if (typeof item === 'string') return item
  return item?.EmailAddress || item?.email || item?.address || ''
}

function normalizedAddress(value) {
  return String(value || '')
    .trim()
    .toLowerCase()
}

async function loadOriginal() {
  const id = Number(route.query.email || 0)
  if (!id) return
  loadingOriginal.value = true
  forwardEmailId.value = id
  try {
    const res = await api.mail.detail(id)
    const detail = unwrap(res)
    const sender = addressValue(detail?.from_address || detail?.from) || route.query.to || ''
    const originalTo = parseAddressList(detail?.to).map(addressValue).filter(Boolean)
    const originalCc = parseAddressList(detail?.cc).map(addressValue).filter(Boolean)
    const ownAddresses = new Set(
      addresses.value.map((item) => normalizedAddress(addressValue(item))).filter(Boolean)
    )
    const excludeOwn = (items) =>
      [...new Set(items.map((item) => String(item).trim()).filter(Boolean))].filter(
        (item) => !ownAddresses.has(normalizedAddress(item))
      )
    const originalSubject = detail?.subject || ''
    if (mode.value === 'forward') {
      subject.value = originalSubject
        ? `${t('compose_forwardPrefix')} ${originalSubject}`
        : t('compose_forwardPrefix')
      to.value = []
      cc.value = []
    } else if (mode.value === 'replyAll') {
      const replyTo = excludeOwn([sender, ...originalTo])
      const replyCc = excludeOwn(originalCc).filter(
        (item) =>
          !replyTo.some((recipient) => normalizedAddress(recipient) === normalizedAddress(item))
      )
      to.value = replyTo
      cc.value = replyCc
      subject.value = originalSubject
        ? `${t('compose_replyPrefix')} ${originalSubject}`
        : t('compose_replyPrefix')
    } else {
      to.value = sender ? [sender] : []
      cc.value = []
      subject.value = originalSubject
        ? `${t('compose_replyPrefix')} ${originalSubject}`
        : t('compose_replyPrefix')
    }
    const quote = detail?.text?.String || detail?.text || detail?.html?.String || detail?.html || ''
    editor.value?.commands.setContent(
      `<blockquote>${quoteText(quote).replaceAll('\n', '<br>')}</blockquote><p></p>`
    )
  } catch (e) {
    error.value = e?.response?.data?.errorMsg || t('common_loadFailed')
  } finally {
    loadingOriginal.value = false
  }
}

function restoreDraft() {
  if (mode.value !== 'compose') return
  const raw = localStorage.getItem(draftKey)
  if (!raw) return
  try {
    const draft = JSON.parse(raw)
    if (!window.confirm(t('compose_restoreDraft'))) return
    to.value = draft.to || []
    cc.value = draft.cc || []
    bcc.value = draft.bcc || []
    subject.value = draft.subject || ''
    from.value = draft.from || from.value
    editor.value?.commands.setContent(draft.html || '<p></p>')
  } catch {
    localStorage.removeItem(draftKey)
  }
}

async function attach(event) {
  const files = Array.from(event.target.files || [])
  const encoded = await Promise.all(
    files.map(
      (file) =>
        new Promise((resolve) => {
          const reader = new FileReader()
          reader.onload = () => resolve({ file, data: reader.result })
          reader.readAsDataURL(file)
        })
    )
  )
  attachments.value.push(...encoded)
  event.target.value = ''
}

function removeAttachment(item) {
  attachments.value = attachments.value.filter((entry) => entry !== item)
}

function draftData() {
  return {
    to: to.value,
    cc: cc.value,
    bcc: bcc.value,
    subject: subject.value,
    from: from.value,
    html: editor.value?.getHTML() || ''
  }
}

function setLink() {
  const current = editor.value?.getAttributes('link').href || ''
  const url = window.prompt(t('compose.linkPrompt'), current)
  if (url === null) return
  if (!url.trim()) editor.value?.chain().focus().unsetLink().run()
  else editor.value?.chain().focus().setLink({ href: url.trim() }).run()
}

function saveDraft() {
  localStorage.setItem(draftKey, JSON.stringify(draftData()))
  message.success(t('compose_draftSaved'))
}

async function submit() {
  if (!to.value.length || !subject.value) {
    message.error(t('compose_required'))
    return
  }
  sending.value = true
  error.value = ''
  deliveryResults.value = []
  try {
    const html = editor.value?.getHTML() || ''
    const text = editor.value?.getText() || ''
    const payload = {
      from_address: from.value,
      to: to.value,
      cc: cc.value,
      text,
      html,
      include_attachments: includeAttachments.value
    }
    let res
    if (isForward.value) {
      res = await api.mail.forward({
        ...payload,
        email_id: forwardEmailId.value,
        bcc: bcc.value
      })
    } else {
      res = await api.mail.send({
        from: { email: from.value, name: auth.user?.name || '' },
        from_address: from.value,
        to: to.value.map((email) => ({ email })),
        cc: cc.value.map((email) => ({ email })),
        bcc: bcc.value.map((email) => ({ email })),
        subject: subject.value,
        html,
        text,
        attrs: attachments.value.map((item) => ({ name: item.file.name, data: item.data }))
      })
    }
    const data = unwrap(res, {}) || {}
    deliveryResults.value = Array.isArray(data.recipients) ? data.recipients : []
    if (res.errorNo || data.status === 'failed') {
      error.value = res.errorMsg || t('compose_sendFailed')
      message.error(error.value)
      return
    }
    localStorage.removeItem(draftKey)
    message.success(t('compose_submitted'))
    if (!deliveryResults.value.length) router.replace('/mail')
  } catch (e) {
    error.value = e?.response?.data?.errorMsg || e?.message || t('compose_sendFailed')
    message.error(error.value)
  } finally {
    sending.value = false
  }
}

onMounted(async () => {
  await loadAddresses()
  await loadOriginal()
  restoreDraft()
})
onBeforeUnmount(() => editor.value?.destroy())
</script>

<template>
  <section class="compose-page">
    <n-card class="compose-card" :bordered="false">
      <header class="compose-header">
        <n-button quaternary circle :aria-label="t('common_back')" @click="router.back()">
          <n-icon><ArrowBackOutline /></n-icon>
        </n-button>
        <div>
          <h1>{{ isForward ? t('common_forward') : t('compose_title') }}</h1>
          <span>{{ loadingOriginal ? t('common_loading') : t('compose_subtitle') }}</span>
        </div>
        <div class="compose-header-actions">
          <n-button secondary @click="router.back()">
            <n-icon><CloseOutline /></n-icon>{{ t('common_cancel') }}
          </n-button>
          <n-button secondary @click="saveDraft">
            <n-icon><SaveOutline /></n-icon>{{ t('compose_saveDraft') }}
          </n-button>
          <n-button type="primary" :loading="sending" @click="submit">
            <n-icon><SendOutline /></n-icon>{{ t('common_send') }}
          </n-button>
        </div>
      </header>
      <n-alert v-if="error" type="error" :bordered="false">{{ error }}</n-alert>
      <n-form class="compose-form">
        <n-form-item :label="t('common_from')">
          <n-select v-model:value="from" :options="addressOptions" />
        </n-form-item>
        <n-form-item :label="t('common_to')">
          <n-select
            v-model:value="to"
            multiple
            tag
            filterable
            :options="recipientOptions"
            :placeholder="t('compose_addressPlaceholder')"
          />
        </n-form-item>
        <n-form-item :label="t('common_cc')">
          <n-select v-model:value="cc" multiple tag filterable :options="recipientOptions" />
        </n-form-item>
        <n-form-item :label="t('common_bcc')">
          <n-select v-model:value="bcc" multiple tag filterable :options="recipientOptions" />
        </n-form-item>
        <n-form-item :label="t('common_subject')">
          <n-input v-model:value="subject" :placeholder="t('compose_subjectPlaceholder')" />
        </n-form-item>
      </n-form>
      <div class="editor-toolbar">
        <n-button text @click="editor?.chain().focus().toggleBold().run()"
          ><strong>B</strong></n-button
        >
        <n-button text @click="editor?.chain().focus().toggleItalic().run()"><em>I</em></n-button>
        <n-button text @click="editor?.chain().focus().toggleUnderline().run()"><u>U</u></n-button>
        <n-button text :aria-label="t('compose.link')" @click="setLink"
          ><n-icon><LinkOutline /></n-icon
        ></n-button>
      </div>
      <editor-content v-if="editor" :editor="editor" class="tiptap-editor" />
      <footer class="compose-footer">
        <label class="attach-button">
          <input type="file" multiple @change="attach" />
          <n-icon><AttachOutline /></n-icon>{{ t('compose_addAttachment') }}
        </label>
        <n-tag
          v-for="item in attachments"
          :key="item.file.name"
          closable
          @close="removeAttachment(item)"
        >
          {{ item.file.name }} ({{ Math.ceil(item.file.size / 1024) }} KB)
        </n-tag>
        <n-switch v-if="isForward" v-model:value="includeAttachments">
          {{ t('compose_includeAttachments') }}
        </n-switch>
        <span class="compose-hint">{{ t('compose_queueHint') }}</span>
      </footer>
      <n-space v-if="deliveryResults.length" vertical class="delivery-results">
        <n-alert
          v-for="result in deliveryResults"
          :key="result.address"
          :type="
            result.status === 'failed'
              ? 'error'
              : result.status === 'queued'
                ? 'warning'
                : 'success'
          "
        >
          {{ result.address }} · {{ t(`compose_status_${result.status}`)
          }}<span v-if="result.error"> · {{ result.error }}</span>
        </n-alert>
      </n-space>
    </n-card>
  </section>
</template>
