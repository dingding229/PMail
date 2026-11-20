<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted, h } from 'vue'
import {
  NCard,
  NDataTable,
  NButton,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NTag,
  NProgress,
  useMessage,
  useDialog
} from 'naive-ui'
import { userApi } from '@/api'
const rows = ref([]),
  show = ref(false),
  form = ref({}),
  message = useMessage(),
  dialog = useDialog()
async function load() {
  const res = await userApi.list()
  if (!res.errorNo) rows.value = res.data?.list || res.data || []
}
function open(row) {
  form.value = {
    ...row,
    display_name: row.display_name || row.name || '',
    username: row.username || row.name || ''
  }
  show.value = true
}
async function save() {
  const fn = form.value.id ? userApi.update : userApi.create
  const payload = { ...form.value, display_name: form.value.display_name || form.value.username }
  const res = await fn(payload)
  if (res.errorNo) message.error(res.errorMsg)
  else {
    show.value = false
    load()
  }
}
async function reset() {
  const res = await userApi.resetPassword({ id: form.value.id, password: form.value.password })
  if (res.errorNo) message.error(res.errorMsg)
  else message.success(i18n.global.t('legacy_d9de459573'))
}
async function disable(row) {
  dialog.warning({
    title: i18n.global.t('legacy_638157d932'),
    content: i18n.global.t('legacy_f31aa4a7e0'),
    positiveText: i18n.global.t('legacy_b56d9ac6c5'),
    negativeText: i18n.global.t('legacy_4d0b4688c7'),
    onPositiveClick: async () => {
      const res = await userApi.remove(row.id)
      if (res.errorNo) message.error(res.errorMsg)
      else load()
    }
  })
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_baf84751a2')" class="settings-card"
    ><template #header-extra
      ><n-button type="primary" @click="open({})">{{ $t('legacy_315315d695') }}</n-button></template
    ><n-data-table
      :data="rows"
      :columns="[
        { title: $t('legacy_9013849179'), key: 'account' },
        { title: $t('legacy_1be7ae4fc2'), key: 'name' },
        {
          title: $t('legacy_67d2d7970f'),
          key: 'addresses',
          render: (r) => (r.addresses || []).map((x) => x.address).join(', ')
        },
        {
          title: $t('legacy_cecb406cf6'),
          key: 'used_bytes',
          render: (r) =>
            h(
              NProgress,
              {
                type: 'line',
                percentage: r.quota_mb
                  ? Math.min(
                      100,
                      Math.round(((r.used_bytes || 0) / (r.quota_mb * 1024 * 1024)) * 100)
                    )
                  : 0
              },
              () => (r.quota_mb ? `${r.used_mb || 0}/${r.quota_mb}MB` : $t('legacy_09c4fc252c'))
            )
        },
        {
          title: i18n.global.t('legacy_4ce78e8185'),
          key: 'disabled',
          render: (r) =>
            h(NTag, { type: r.disabled ? 'error' : 'success' }, () =>
              r.disabled ? $t('legacy_be70be5a2e') : $t('legacy_d4e9ca3dd4')
            )
        },
        {
          title: $t('legacy_f3ea6d345e'),
          key: 'id',
          render: (r) =>
            h(NButton, { size: 'small', onClick: () => open(r) }, () =>
              i18n.global.t('legacy_08d7bb4a6a')
            )
        }
      ]"
    /><n-modal
      v-model:show="show"
      preset="card"
      :title="$t('legacy_9ba763ea34')"
      style="width: 560px"
      ><n-form label-placement="top"
        ><n-form-item :label="$t('legacy_9013849179')"
          ><n-input v-model:value="form.account" :disabled="!!form.id" /></n-form-item
        ><n-form-item :label="$t('legacy_1be7ae4fc2')"
          ><n-input v-model:value="form.username" /></n-form-item
        ><n-form-item :label="$t('legacy_c839a8ff17')"
          ><n-input v-model:value="form.password" type="password" /></n-form-item
        ><n-form-item :label="$t('legacy_75402d4284')"
          ><n-input-number v-model:value="form.quota_mb" :min="0" /></n-form-item
        ><n-button type="primary" @click="save">{{ $t('legacy_fadf24dbc5') }}</n-button
        ><n-button v-if="form.id" secondary @click="reset">{{ $t('legacy_7e422146dd') }}</n-button
        ><n-button v-if="form.id" tertiary type="warning" @click="disable(form)">{{
          form.disabled ? $t('legacy_d4e9ca3dd4') : $t('legacy_be70be5a2e')
        }}</n-button></n-form
      ></n-modal
    ></n-card
  >
</template>
