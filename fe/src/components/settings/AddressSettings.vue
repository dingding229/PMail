<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted, h } from 'vue'
import {
  NCard,
  NDataTable,
  NButton,
  NSwitch,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NSelect,
  NSpace,
  useMessage,
  useDialog
} from 'naive-ui'
import { addressApi, domainApi } from '@/api'
const props = defineProps({ userId: { type: Number, default: 0 } })
const message = useMessage(),
  dialog = useDialog(),
  rows = ref([]),
  show = ref(false),
  form = ref({}),
  loading = ref(false),
  domains = ref([])
const columns = [
  { title: i18n.global.t('legacy_67d2d7970f'), key: 'address' },
  { title: i18n.global.t('legacy_282d68464d'), key: 'domain' },
  {
    title: i18n.global.t('legacy_160d90890b'),
    key: 'is_primary',
    render: (row) => (row.is_primary ? i18n.global.t('legacy_30160a21b9') : '')
  },
  {
    title: i18n.global.t('legacy_d6aa06353d'),
    key: 'can_send',
    render: (row) => h(NSwitch, { value: row.can_send, onUpdateValue: (v) => updateSend(row, v) })
  },
  {
    title: i18n.global.t('legacy_f3ea6d345e'),
    key: 'action',
    render: (row) =>
      h(NSpace, null, {
        default: () => [
          h(NButton, { size: 'small', disabled: row.is_primary, onClick: () => primary(row) }, () =>
            i18n.global.t('legacy_160d90890b')
          ),
          h(
            NButton,
            { size: 'small', type: 'error', disabled: row.is_primary, onClick: () => remove(row) },
            () => i18n.global.t('legacy_3755f56f2f')
          )
        ]
      })
  }
]
async function load() {
  const [res, domainRes] = await Promise.all([addressApi.list(props.userId), domainApi.list()])
  if (!res.errorNo) rows.value = res.data || []
  if (!domainRes.errorNo) domains.value = domainRes.data || []
}
async function updateSend(row, value) {
  const res = await addressApi.update({ id: row.id, can_send: value })
  if (res.errorNo) message.error(res.errorMsg)
  else row.can_send = value
}
async function primary(row) {
  const res = await addressApi.primary(row.id)
  if (res.errorNo) message.error(res.errorMsg)
  else {
    message.success(i18n.global.t('legacy_4c7345427b'))
    load()
  }
}
async function remove(row) {
  if (row.is_primary) {
    message.warning(i18n.global.t('legacy_ed97483da2'))
    return
  }
  dialog.warning({
    title: i18n.global.t('legacy_d331304db2'),
    content: i18n.global.t('legacy_383f480368'),
    positiveText: i18n.global.t('legacy_3755f56f2f'),
    negativeText: i18n.global.t('legacy_4d0b4688c7'),
    onPositiveClick: async () => {
      const res = await addressApi.remove(row.id)
      if (res.errorNo) message.error(res.errorMsg)
      else load()
    }
  })
}
async function create() {
  loading.value = true
  const res = await addressApi.create({ ...form.value, user_id: props.userId || undefined })
  loading.value = false
  if (res.errorNo) message.error(res.errorMsg)
  else {
    show.value = false
    form.value = {}
    load()
  }
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_e02c9d6103')" class="settings-card">
    <template #header-extra
      ><n-button type="primary" @click="show = true">{{
        $t('legacy_8c2fef26e6')
      }}</n-button></template
    >
    <n-data-table :columns="columns" :data="rows" />
    <n-modal
      v-model:show="show"
      preset="card"
      :title="$t('legacy_8c2fef26e6')"
      style="width: 460px"
    >
      <n-form label-placement="top"
        ><n-form-item :label="$t('legacy_694cf67796')"
          ><n-input
            v-model:value="form.local_part"
            :placeholder="$t('placeholderAlias')" /></n-form-item
        ><n-form-item :label="$t('legacy_282d68464d')">
          <n-select
            v-model:value="form.domain_id"
            :options="domains.map((item) => ({ label: item.name, value: item.id }))"
          /> </n-form-item
        ><n-button type="primary" :loading="loading" @click="create">{{
          $t('legacy_fcbd093292')
        }}</n-button></n-form
      >
    </n-modal>
  </n-card>
</template>
