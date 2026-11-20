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
  NSelect,
  NSwitch,
  NTag,
  NSpace,
  NAlert,
  useMessage,
  useDialog
} from 'naive-ui'
import { externalApi } from '@/api'
const rows = ref([]),
  presets = ref([]),
  show = ref(false),
  form = ref({
    protocol: 'imap',
    tls_mode: 'tls',
    port: 993,
    folder: i18n.global.t('folderDefault'),
    interval_minutes: 10,
    initial_days: 30,
    delete_remote: false,
    insecure_skip_verify: false
  }),
  loading = ref(false),
  error = ref(''),
  message = useMessage(),
  dialog = useDialog()
const protocolOptions = [
    { label: i18n.global.t('protocolImap'), value: 'imap' },
    { label: i18n.global.t('protocolPop3'), value: 'pop3' }
  ],
  tlsOptions = [
    { label: 'TLS', value: 'tls' },
    { label: 'STARTTLS', value: 'starttls' },
    { label: i18n.global.t('legacy_f69a12f88e'), value: 'none' }
  ]
async function load() {
  const [list, preset] = await Promise.all([externalApi.list(), externalApi.presets()])
  if (!list.errorNo) rows.value = list.data || []
  presets.value = preset.data || []
}
function open(row) {
  form.value = row
    ? { ...row, password: '' }
    : {
        protocol: 'imap',
        tls_mode: 'tls',
        port: 993,
        folder: i18n.global.t('folderDefault'),
        interval_minutes: 10,
        initial_days: 30,
        delete_remote: false,
        insecure_skip_verify: false
      }
  show.value = true
}
function preset(name) {
  const p = presets.value.find((x) => x.name === name)
  if (p) form.value = { ...form.value, ...p }
}
async function save() {
  loading.value = true
  error.value = ''
  try {
    const res = form.value.id
      ? await externalApi.update(form.value)
      : await externalApi.create(form.value)
    if (res.errorNo) throw new Error(res.errorMsg)
    show.value = false
    message.success(i18n.global.t('legacy_21bd2818af'))
    load()
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
async function test() {
  loading.value = true
  const res = await externalApi.test(form.value)
  loading.value = false
  if (res.errorNo) message.error(res.errorMsg)
  else
    message.success(
      `${i18n.global.t('external_testSuccess')}${res.data?.folders?.join(', ') || i18n.global.t('folderDefault')}`
    )
}
async function sync(id) {
  const res = await externalApi.sync(id)
  if (res.errorNo) message.error(res.errorMsg)
  else message.success(i18n.global.t('legacy_ac8c362a4d'))
}
async function remove(row) {
  dialog.warning({
    title: i18n.global.t('legacy_cf31352deb'),
    content: i18n.global.t('legacy_a5d30120fd'),
    positiveText: i18n.global.t('legacy_3755f56f2f'),
    negativeText: i18n.global.t('legacy_4d0b4688c7'),
    onPositiveClick: async () => {
      const res = await externalApi.remove(row.id)
      if (res.errorNo) message.error(res.errorMsg)
      else load()
    }
  })
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_5bdba1e468')" class="settings-card"
    ><template #header-extra
      ><n-button type="primary" @click="open()">{{ $t('legacy_14ac719f93') }}</n-button></template
    ><n-data-table
      :data="rows"
      :columns="[
        { title: $t('legacy_1be7ae4fc2'), key: 'name' },
        { title: $t('legacy_b0c431675b'), key: 'protocol' },
        { title: $t('legacy_30d94cb3b9'), key: 'host' },
        { title: i18n.global.t('legacy_7ca4e2e0b9'), key: 'last_sync_at' },
        {
          title: i18n.global.t('legacy_4ce78e8185'),
          key: 'enabled',
          render: (r) =>
            h(NTag, { type: r.enabled ? 'success' : 'warning' }, () =>
              r.enabled ? $t('legacy_d4e9ca3dd4') : i18n.global.t('legacy_be70be5a2e')
            )
        },
        { title: i18n.global.t('common_error'), key: 'last_error' },
        {
          title: $t('legacy_f3ea6d345e'),
          key: 'id',
          render: (r) =>
            h(NSpace, null, {
              default: () => [
                h(NButton, { size: 'small', onClick: () => sync(r.id) }, () =>
                  i18n.global.t('legacy_ac8c362a4d')
                ),
                h(NButton, { size: 'small', onClick: () => open(r) }, () =>
                  i18n.global.t('common_edit')
                ),
                h(NButton, { size: 'small', type: 'error', onClick: () => remove(r) }, () =>
                  $t('legacy_3755f56f2f')
                )
              ]
            })
        }
      ]"
    /><n-modal
      v-model:show="show"
      preset="card"
      :title="$t('legacy_5bdba1e468')"
      style="width: 680px"
      ><n-alert v-if="error" type="error">{{ error }}</n-alert
      ><n-form label-placement="top" label-width="120"
        ><n-form-item :label="$t('legacy_87011af9db')"
          ><n-select
            clearable
            :options="presets.map((p) => ({ label: p.name, value: p.name }))"
            @update:value="preset" /></n-form-item
        ><n-form-item :label="$t('legacy_1be7ae4fc2')"
          ><n-input v-model:value="form.name" /></n-form-item
        ><n-form-item :label="$t('legacy_b0c431675b')"
          ><n-select v-model:value="form.protocol" :options="protocolOptions" /></n-form-item
        ><n-form-item :label="$t('legacy_30d94cb3b9')"
          ><n-input v-model:value="form.host" /></n-form-item
        ><n-form-item :label="$t('legacy_6cbb7335b5')"
          ><n-input-number v-model:value="form.port" :min="1" :max="65535" /></n-form-item
        ><n-form-item :label="$t('legacy_2e85d63c00')"
          ><n-select v-model:value="form.tls_mode" :options="tlsOptions" /></n-form-item
        ><n-form-item :label="$t('legacy_a1aaf352cb')"
          ><n-input v-model:value="form.username" /></n-form-item
        ><n-form-item :label="$t('legacy_8a2332501b')"
          ><n-input
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :placeholder="$t('legacy_cbdf0b1f3f')" /></n-form-item
        ><n-form-item :label="$t('legacy_46ecac2910')"
          ><n-input
            v-model:value="form.folder"
            :placeholder="$t('folderPlaceholder')" /></n-form-item
        ><n-form-item :label="$t('legacy_a2a1c72238')"
          ><n-input-number v-model:value="form.initial_days" :min="0" /></n-form-item
        ><n-form-item :label="$t('legacy_9bbca16c8e')"
          ><n-input-number v-model:value="form.interval_minutes" :min="2" /></n-form-item
        ><n-form-item :label="$t('legacy_e51d2e6b30')"
          ><n-switch v-model:value="form.delete_remote" /></n-form-item
        ><n-form-item :label="$t('legacy_91ae87a36d')"
          ><n-switch v-model:value="form.insecure_skip_verify" /><span class="warning-text">{{
            $t('legacy_89342db5ca')
          }}</span></n-form-item
        ><n-space
          ><n-button :loading="loading" @click="test">{{ $t('legacy_10b7d8eccc') }}</n-button
          ><n-button type="primary" :loading="loading" @click="save">{{
            $t('legacy_fadf24dbc5')
          }}</n-button></n-space
        ></n-form
      ></n-modal
    ></n-card
  >
</template>
