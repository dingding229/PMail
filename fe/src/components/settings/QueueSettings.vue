<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted, h } from 'vue'
import { NCard, NDataTable, NButton, NTag, useMessage, useDialog } from 'naive-ui'
import { queueApi } from '@/api'
const rows = ref([]),
  message = useMessage(),
  dialog = useDialog()
async function load() {
  const res = await queueApi.list()
  if (!res.errorNo) rows.value = res.data || []
}
async function retry(row) {
  const res = await queueApi.retry(row.id)
  if (res.errorNo) message.error(res.errorMsg)
  else {
    message.success(i18n.global.t('legacy_a3477f8492'))
    load()
  }
}
async function remove(row) {
  dialog.warning({
    title: i18n.global.t('legacy_cf35efdef1'),
    content: i18n.global.t('legacy_574e45d30f'),
    positiveText: i18n.global.t('legacy_3755f56f2f'),
    negativeText: i18n.global.t('legacy_4d0b4688c7'),
    onPositiveClick: async () => {
      const res = await queueApi.remove(row.id)
      if (res.errorNo) message.error(res.errorMsg)
      else load()
    }
  })
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_f3ef3addc6')" class="settings-card"
    ><n-data-table
      :data="rows"
      :columns="[
        { title: $t('legacy_529414cfe5'), key: 'rcpt_to' },
        {
          title: i18n.global.t('legacy_4ce78e8185'),
          key: 'status',
          render: (r) =>
            h(
              NTag,
              {
                type: r.status === 'failed' ? 'error' : r.status === 'sent' ? 'success' : 'warning'
              },
              () => r.status
            )
        },
        { title: i18n.global.t('legacy_4b176c98de'), key: 'attempts' },
        { title: i18n.global.t('legacy_8e9d7c5b49'), key: 'next_retry_at' },
        { title: i18n.global.t('legacy_9b1b8f2b65'), key: 'last_error' },
        {
          title: $t('legacy_f3ea6d345e'),
          key: 'id',
          render: (r) =>
            h(NButton, { size: 'small', onClick: () => retry(r) }, () =>
              i18n.global.t('legacy_7f5a9b3e0c')
            )
        }
      ]"
  /></n-card>
</template>
