<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted } from 'vue'
import { NCard, NForm, NFormItem, NSwitch, NSelect, NButton, NAlert, useMessage } from 'naive-ui'
import { forwardApi } from '@/api'
const value = ref({ enabled: false, targets: [], keep_copy: true }),
  error = ref(''),
  message = useMessage(),
  loading = ref(false)
async function load() {
  const res = await forwardApi.get()
  if (res.errorNo) error.value = res.errorMsg
  else value.value = { ...(res.data?.setting || {}), targets: res.data?.targets || [] }
}
async function save() {
  loading.value = true
  const res = await forwardApi.set(value.value)
  loading.value = false
  if (res.errorNo) error.value = res.errorMsg
  else message.success(i18n.global.t('legacy_ef72db77a5'))
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_50cc88091b')" class="settings-card"
    ><n-alert v-if="error" type="error">{{ error }}</n-alert
    ><n-form label-placement="top" label-width="120"
      ><n-form-item :label="$t('legacy_d4e9ca3dd4')"
        ><n-switch v-model:value="value.enabled" /></n-form-item
      ><n-form-item :label="$t('legacy_a339fefd4a')"
        ><n-select
          v-model:value="value.targets"
          multiple
          tag
          filterable
          :placeholder="$t('legacy_51175246e9')" /></n-form-item
      ><n-form-item :label="$t('legacy_12ff5ffc7a')"
        ><n-switch v-model:value="value.keep_copy" /></n-form-item
      ><n-button type="primary" :loading="loading" @click="save">{{
        $t('legacy_bb79ec7c15')
      }}</n-button></n-form
    ></n-card
  >
</template>
