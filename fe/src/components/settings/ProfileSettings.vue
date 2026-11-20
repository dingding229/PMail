<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted } from 'vue'
import { NCard, NForm, NFormItem, NInput, NButton, NAlert, useMessage } from 'naive-ui'
import { userApi } from '@/api'
const message = useMessage()
const profile = ref({})
const password = ref({ old_password: '', new_password: '' })
const loading = ref(false)
const error = ref('')
async function load() {
  const res = await userApi.me()
  if (res.errorNo) error.value = res.errorMsg
  else profile.value = res.data || {}
}
async function save() {
  loading.value = true
  error.value = ''
  try {
    const res = await userApi.me({ display_name: profile.value.name })
    if (res.errorNo) throw new Error(res.errorMsg)
    message.success(i18n.global.t('legacy_7e68eb622d'))
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
async function changePassword() {
  loading.value = true
  error.value = ''
  try {
    const res = await userApi.changePassword(password.value)
    if (res.errorNo) throw new Error(res.errorMsg)
    password.value = { old_password: '', new_password: '' }
    message.success(i18n.global.t('legacy_be60abcc48'))
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>
<template>
  <div class="settings-stack">
    <n-alert v-if="error" type="error" :bordered="false">{{ error }}</n-alert>
    <n-card :title="$t('legacy_d562e344b9')" class="settings-card">
      <n-form label-placement="top" label-width="90">
        <n-form-item :label="$t('legacy_9013849179')"
          ><n-input :value="profile.account" disabled
        /></n-form-item>
        <n-form-item :label="$t('legacy_c10bbf5ddd')"
          ><n-input v-model:value="profile.name"
        /></n-form-item>
        <n-form-item :label="$t('legacy_cecb406cf6')"
          ><span
            >{{ profile.used_mb || 0 }} MB / {{ profile.quota_mb || $t('legacy_09c4fc252c') }}</span
          ></n-form-item
        >
        <n-button type="primary" :loading="loading" @click="save">{{
          $t('legacy_33f24b381b')
        }}</n-button>
      </n-form>
    </n-card>
    <n-card :title="$t('legacy_0ae20ac3af')" class="settings-card">
      <n-form label-placement="top" label-width="90">
        <n-form-item :label="$t('legacy_8a4f13a03c')"
          ><n-input v-model:value="password.old_password" type="password" show-password-on="click"
        /></n-form-item>
        <n-form-item :label="$t('legacy_d22c9c0085')"
          ><n-input v-model:value="password.new_password" type="password" show-password-on="click"
        /></n-form-item>
        <n-button type="primary" :loading="loading" @click="changePassword">{{
          $t('legacy_0ae20ac3af')
        }}</n-button>
      </n-form>
    </n-card>
  </div>
</template>
