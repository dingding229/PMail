<script setup>
import { ref, onMounted, h, computed } from 'vue'
import {
  NCard,
  NDataTable,
  NButton,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NSelect,
  NSwitch,
  useMessage
} from 'naive-ui'
import { userApi, addressApi, domainApi } from '@/api'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const users = ref([])
const domains = ref([])
const addresses = ref([])
const show = ref(false)
const loading = ref(false)
const form = ref({ user_id: null, domain_id: null, local_part: '', can_send: true })
const message = useMessage()
const userOptions = computed(() =>
  users.value.map((item) => ({ label: item.display_name || item.account, value: item.id }))
)
const domainOptions = computed(() =>
  domains.value.map((item) => ({ label: item.name, value: item.id }))
)
const columns = computed(() => [
  { title: t('common_address'), key: 'address' },
  { title: t('common_user'), key: 'user' },
  {
    title: t('common_primary'),
    key: 'is_primary',
    render: (row) => (row.is_primary ? t('common_yes') : '')
  },
  {
    title: t('common_canSend'),
    key: 'can_send',
    render: (row) =>
      h(NSwitch, { value: row.can_send, onUpdateValue: (value) => update(row, value) })
  }
])

async function load() {
  const [userResponse, domainResponse] = await Promise.all([userApi.list(), domainApi.list()])
  users.value = userResponse.data?.list || userResponse.data || []
  domains.value = domainResponse.data || []
  addresses.value = users.value.flatMap((user) =>
    (user.addresses || []).map((item) => ({ ...item, user: user.account, user_id: user.id }))
  )
}
async function update(row, canSend) {
  const res = await addressApi.update({ id: row.id, can_send: canSend })
  if (res.errorNo) message.error(res.errorMsg)
  else row.can_send = canSend
}
async function add() {
  if (!form.value.user_id || !form.value.domain_id || !form.value.local_part) {
    message.error(t('address_required'))
    return
  }
  loading.value = true
  const res = await addressApi.create(form.value)
  loading.value = false
  if (res.errorNo) message.error(res.errorMsg)
  else {
    message.success(t('common_created'))
    show.value = false
    form.value = { user_id: null, domain_id: null, local_part: '', can_send: true }
    await load()
  }
}
onMounted(load)
</script>

<template>
  <n-card :title="t('admin_addresses')" class="settings-card">
    <template #header-extra
      ><n-button type="primary" @click="show = true">{{
        t('address_addAlias')
      }}</n-button></template
    >
    <n-data-table :data="addresses" :columns="columns" />
    <n-modal
      v-model:show="show"
      preset="card"
      :title="t('address_addForUser')"
      style="width: 460px"
    >
      <n-form label-placement="top">
        <n-form-item :label="t('common_user')"
          ><n-select v-model:value="form.user_id" :options="userOptions"
        /></n-form-item>
        <n-form-item :label="t('common_domain')"
          ><n-select v-model:value="form.domain_id" :options="domainOptions"
        /></n-form-item>
        <n-form-item :label="t('address_localPart')"
          ><n-input v-model:value="form.local_part" :placeholder="$t('placeholderAlias')"
        /></n-form-item>
        <n-form-item :label="t('common_canSend')"
          ><n-switch v-model:value="form.can_send"
        /></n-form-item>
        <n-button type="primary" :loading="loading" @click="add">{{ t('common_create') }}</n-button>
      </n-form>
    </n-modal>
  </n-card>
</template>
