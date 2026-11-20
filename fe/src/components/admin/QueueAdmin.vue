<script setup>
import { ref, onMounted, h } from 'vue'
import { NCard, NDataTable, NButton, NTag, useMessage } from 'naive-ui'
import { queueApi } from '@/api'
const rows = ref([]),
  message = useMessage()
async function load() {
  const r = await queueApi.list()
  rows.value = r.data || []
}
async function retry(row) {
  const r = await queueApi.retry(row.id)
  if (r.errorNo) message.error(r.errorMsg)
  else load()
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_048fd56e7c')" class="settings-card"
    ><n-data-table
      :data="rows"
      :columns="[
        { title: $t('legacy_9ba763ea34'), key: 'user_id' },
        { title: $t('legacy_529414cfe5'), key: 'rcpt_to' },
        {
          title: $t('legacy_4ce78e8185'),
          key: 'status',
          render: (r) => h(NTag, { type: r.status === 'failed' ? 'error' : 'info' }, () => r.status)
        },
        { title: $t('legacy_4b176c98de'), key: 'attempts' },
        { title: $t('legacy_9b1b8f2b65'), key: 'last_error' },
        {
          title: $t('legacy_f3ea6d345e'),
          key: 'id',
          render: (r) =>
            h(NButton, { size: 'small', onClick: () => retry(r) }, () => $t('legacy_7f5a9b3e0c'))
        }
      ]"
  /></n-card>
</template>
