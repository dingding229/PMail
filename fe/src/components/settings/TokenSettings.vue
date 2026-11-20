<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted, h } from 'vue'
import {
  NCard,
  NDataTable,
  NButton,
  NModal,
  NInput,
  NForm,
  NFormItem,
  NAlert,
  useMessage,
  useDialog
} from 'naive-ui'
import { tokenApi } from '@/api'
const rows = ref([]),
  show = ref(false),
  created = ref(''),
  name = ref(''),
  message = useMessage(),
  dialog = useDialog()
async function load() {
  const res = await tokenApi.list()
  if (!res.errorNo) rows.value = res.data || []
}
async function create() {
  const res = await tokenApi.create({ name: name.value || 'Web token' })
  if (res.errorNo) message.error(res.errorMsg)
  else {
    created.value = res.data?.token || ''
    show.value = true
    load()
  }
}
async function remove(row) {
  dialog.warning({
    title: i18n.global.t('legacy_1d0c1c2b3f'),
    content: i18n.global.t('legacy_cc6739225c'),
    positiveText: i18n.global.t('legacy_3755f56f2f'),
    negativeText: i18n.global.t('legacy_4d0b4688c7'),
    onPositiveClick: async () => {
      const res = await tokenApi.remove(row.id)
      if (res.errorNo) message.error(res.errorMsg)
      else load()
    }
  })
}
function openCreate() {
  show.value = true
  created.value = ''
  name.value = ''
}
async function copy() {
  await navigator.clipboard.writeText(created.value)
  message.success(i18n.global.t('legacy_e381a5763d'))
}
onMounted(load)
</script>
<template>
  <n-card title="API Token" class="settings-card"
    ><template #header-extra
      ><n-button type="primary" @click="openCreate">{{
        $t('legacy_51d1c27d92')
      }}</n-button></template
    ><n-data-table
      :data="rows"
      :columns="[
        { title: $t('legacy_1be7ae4fc2'), key: 'name' },
        { title: i18n.global.t('legacy_0d746c8e2f'), key: 'prefix' },
        { title: i18n.global.t('legacy_7ca4e2e0b9'), key: 'last_used_at' },
        { title: i18n.global.t('legacy_2d1a7e4c8f'), key: 'expires_at' },
        {
          title: $t('legacy_f3ea6d345e'),
          key: 'id',
          render: (r) =>
            h(NButton, { size: 'small', type: 'error', onClick: () => remove(r) }, () =>
              $t('legacy_3755f56f2f')
            )
        }
      ]"
    /><n-modal v-model:show="show" preset="card" title="API Token" style="width: 520px"
      ><n-alert v-if="created" type="warning" :title="$t('legacy_62edc6ae47')">{{
        $t('legacy_49cfa53a36')
      }}</n-alert
      ><n-form label-placement="top"
        ><n-form-item :label="$t('legacy_1be7ae4fc2')"
          ><n-input v-model:value="name" :disabled="!!created" /></n-form-item
        ><n-input v-if="created" :value="created" readonly /><n-button
          v-if="created"
          type="primary"
          @click="copy"
          >{{ $t('legacy_d0af290509') }}</n-button
        ><n-button v-else type="primary" @click="create">{{
          $t('legacy_fcbd093292')
        }}</n-button></n-form
      ></n-modal
    ></n-card
  >
</template>
