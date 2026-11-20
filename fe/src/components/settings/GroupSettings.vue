<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted, h } from 'vue'
import {
  NCard,
  NDataTable,
  NButton,
  NInput,
  NModal,
  NForm,
  NFormItem,
  useMessage,
  useDialog
} from 'naive-ui'
import { groupApi } from '@/api'
const rows = ref([]),
  show = ref(false),
  name = ref(''),
  message = useMessage(),
  dialog = useDialog()
async function load() {
  const res = await groupApi.list()
  if (!res.errorNo) rows.value = res.data || []
}
async function add() {
  const res = await groupApi.add({ name: name.value })
  if (res.errorNo) message.error(res.errorMsg)
  else {
    show.value = false
    name.value = ''
    load()
  }
}
async function remove(row) {
  dialog.warning({
    title: i18n.global.t('legacy_777f53cc71'),
    content: i18n.global.t('legacy_8667dbf488'),
    positiveText: i18n.global.t('legacy_3755f56f2f'),
    negativeText: i18n.global.t('legacy_4d0b4688c7'),
    onPositiveClick: async () => {
      const res = await groupApi.remove(row.id)
      if (res.errorNo) message.error(res.errorMsg)
      else load()
    }
  })
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_e900b55ce7')" class="settings-card"
    ><template #header-extra
      ><n-button type="primary" @click="show = true">{{
        $t('legacy_ac8e6b0575')
      }}</n-button></template
    ><n-data-table
      :data="rows"
      :columns="[
        { title: $t('legacy_1be7ae4fc2'), key: 'name' },
        {
          title: $t('legacy_f3ea6d345e'),
          key: 'id',
          render: (r) =>
            h(NButton, { size: 'small', type: 'error', onClick: () => remove(r) }, () =>
              $t('legacy_3755f56f2f')
            )
        }
      ]"
    /><n-modal
      v-model:show="show"
      preset="card"
      :title="$t('legacy_ac8e6b0575')"
      style="width: 420px"
      ><n-form label-placement="top"
        ><n-form-item :label="$t('legacy_1be7ae4fc2')"><n-input v-model:value="name" /></n-form-item
        ><n-button type="primary" @click="add">{{ $t('legacy_fcbd093292') }}</n-button></n-form
      ></n-modal
    ></n-card
  >
</template>
