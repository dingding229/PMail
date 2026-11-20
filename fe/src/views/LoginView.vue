<script setup>
import { ref } from 'vue'
import { NCard, NForm, NFormItem, NInput, NButton, NAlert, NSelect, useMessage } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'
import { i18n, setLocale } from '@/i18n/i18n'
const auth = useAuthStore(),
  message = useMessage(),
  account = ref(''),
  password = ref(''),
  loading = ref(false),
  error = ref(''),
  locale = i18n.global.locale
async function login() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(account.value, password.value)
    message.success(i18n.global.t('legacy_2991317aba'))
  } catch (e) {
    error.value = e.message || i18n.global.t('legacy_aeb6c8a818')
  } finally {
    loading.value = false
  }
}
</script>
<template>
  <main class="auth-page">
    <div class="auth-visual">
      <div class="hero-orb orb-one" />
      <div class="hero-orb orb-two" />
      <span class="brand-mark large">P</span>
      <h1>{{ $t('legacy_94328640aa') }}</h1>
      <p>{{ $t('legacy_3abad77f33') }}</p>
    </div>
    <n-card class="auth-card" :bordered="false"
      ><div class="auth-header">
        <span class="brand-mark">P</span>
        <div>
          <h2>{{ $t('legacy_53b9ac25fd') }}</h2>
          <p>{{ $t('legacy_d2b2718322') }}</p>
        </div>
      </div>
      <n-alert v-if="error" type="error" :bordered="false" class="form-alert">{{
        error
      }}</n-alert><n-form @submit.prevent="login"
        ><n-form-item :label="$t('legacy_e6c9e01537')"
          ><n-input
            v-model:value="account"
            size="large"
            placeholder="name@example.com"
            autofocus /></n-form-item
        ><n-form-item :label="$t('legacy_c839a8ff17')"
          ><n-input
            v-model:value="password"
            size="large"
            type="password"
            show-password-on="click"
            :placeholder="$t('legacy_713b738250')"
            @keyup.enter="login" /></n-form-item
        ><n-button type="primary" size="large" block :loading="loading" attr-type="submit">{{
          $t('legacy_21f1e88275')
        }}</n-button></n-form
      >
      <div class="auth-footer">
        <span>{{ $t('legacy_cd99b225a0') }}</span><n-select
          size="small"
          :value="locale"
          :options="[
            { label: i18n.global.t('legacy_3f1b8e5c2a'), value: 'zh' },
            { label: $t('languageEnglish'), value: 'en' }
          ]"
          @update:value="setLocale"
        /></div
    ></n-card>
  </main>
</template>
