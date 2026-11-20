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
  NTag,
  NTable,
  NCode,
  NSpace,
  useMessage,
  useDialog
} from 'naive-ui'
import { domainApi } from '@/api'
const rows = ref([]),
  dns = ref([]),
  show = ref(false),
  name = ref(''),
  message = useMessage(),
  dialog = useDialog()
async function load() {
  const res = await domainApi.list()
  if (!res.errorNo) rows.value = res.data || []
}
async function create() {
  const res = await domainApi.create({ name: name.value })
  if (res.errorNo) message.error(res.errorMsg)
  else {
    show.value = false
    load()
  }
}
async function setDefault(row) {
  const res = await domainApi.primary(row.id)
  if (res.errorNo) message.error(res.errorMsg)
  else await load()
}
async function dnsFor(row) {
  const res = await domainApi.dns(row.id)
  if (res.errorNo) message.error(res.errorMsg)
  else dns.value = res.data || []
}
async function remove(row) {
  dialog.warning({
    title: i18n.global.t('legacy_dd7ffe3279'),
    content: i18n.global.t('legacy_574e45d30f'),
    positiveText: i18n.global.t('legacy_3755f56f2f'),
    negativeText: i18n.global.t('legacy_4d0b4688c7'),
    onPositiveClick: async () => {
      const res = await domainApi.remove(row.id)
      if (res.errorNo) message.error(res.errorMsg)
      else load()
    }
  })
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_96de09f813')" class="settings-card"
    ><template #header-extra
      ><n-button type="primary" @click="show = true">{{
        $t('legacy_87f3a1fb0d')
      }}</n-button></template
    ><n-data-table
      :data="rows"
      :columns="[
        { title: $t('legacy_282d68464d'), key: 'name' },
        {
          title: i18n.global.t('common_default'),
          key: 'is_default',
          render: (r) => (r.is_default ? i18n.global.t('common_default') : '')
        },
        { title: 'DKIM', key: 'dkim_selector' },
        {
          title: 'DNS',
          key: 'id',
          render: (r) =>
            h(NButton, { size: 'small', onClick: () => dnsFor(r) }, () =>
              i18n.global.t('domain_dns')
            )
        },
        {
          title: $t('legacy_f3ea6d345e'),
          key: 'id',
          render: (r) =>
            h(NSpace, null, {
              default: () => [
                h(
                  NButton,
                  { size: 'small', disabled: r.is_default, onClick: () => setDefault(r) },
                  () => i18n.global.t('domain_setDefault')
                ),
                h(NButton, { size: 'small', type: 'error', onClick: () => remove(r) }, () =>
                  $t('legacy_3755f56f2f')
                )
              ]
            })
        }
      ]"
    /><n-table v-if="dns.length" class="dns-table"
      ><thead>
        <tr>
          <th>{{ $t('legacy_e4e46c7235') }}</th>
          <th>{{ $t('legacy_2e8a0cf711') }}</th>
          <th>{{ $t('legacy_6de9c830ac') }}</th>
          <th>{{ $t('legacy_336e5c7eb6') }}</th>
          <th>{{ $t('legacy_c7127bb6ee') }}</th>
          <th>{{ $t('legacy_f3ea6d345e') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in dns" :key="row.type + row.host">
          <td>{{ row.type }}</td>
          <td>{{ row.host }}</td>
          <td>{{ row.value }}</td>
          <td>{{ row.actual || '—' }}</td>
          <td>
            <n-tag :type="row.match ? 'success' : 'warning'">{{
              row.match ? $t('legacy_30160a21b9') : $t('legacy_8bf5c10ad9')
            }}</n-tag>
          </td>
          <td>
            <n-button size="small" @click="navigator.clipboard.writeText(row.value)">{{
              $t('legacy_4edd1d0087')
            }}</n-button>
          </td>
        </tr>
      </tbody></n-table
    ><n-modal
      v-model:show="show"
      preset="card"
      :title="$t('legacy_87f3a1fb0d')"
      style="width: 420px"
      ><n-form label-placement="top"
        ><n-form-item :label="$t('legacy_282d68464d')"><n-input v-model:value="name" /></n-form-item
        ><n-button type="primary" @click="create">{{ $t('legacy_fcbd093292') }}</n-button></n-form
      ></n-modal
    ></n-card
  >
</template>
