<script setup>
import { ref } from 'vue'
import {
  NCard,
  NSteps,
  NStep,
  NForm,
  NFormItem,
  NInput,
  NSelect,
  NButton,
  NAlert,
  useMessage
} from 'naive-ui'
import { api } from '@/api'
import { router } from '@/router'
import { i18n } from '@/i18n/i18n'

const message = useMessage()
const step = ref(1)
const error = ref('')
const loading = ref(false)
const form = ref({
  db_type: 'postgres',
  db_dsn: 'postgres://pmail:password@postgres:5432/pmail?sslmode=disable',
  domain: 'example.com',
  account: 'admin',
  password: ''
})
const dbOptions = [
  { label: i18n.global.t('setup_postgres'), value: 'postgres' },
  { label: i18n.global.t('dbSqlite'), value: 'sqlite' },
  { label: i18n.global.t('dbMysql'), value: 'mysql' }
]
async function save() {
  loading.value = true
  error.value = ''
  try {
    await api.setup({
      action: 'set',
      step: 'database',
      db_type: form.value.db_type,
      db_dsn: form.value.db_dsn
    })
    await api.setup({ action: 'set', step: 'domain', domain: form.value.domain })
    await api.setup({
      action: 'set',
      step: 'account',
      account: form.value.account,
      password: form.value.password
    })
    message.success(i18n.global.t('setup_complete'))
    router.replace('/login')
  } catch (e) {
    error.value = e?.response?.data?.errorMsg || i18n.global.t('setup_failed')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="setup-page">
    <n-card class="setup-card" :bordered="false">
      <div class="setup-brand">
        <span class="brand-mark">P</span>
        <h1>{{ $t('setup_title') }}</h1>
      </div>
      <n-steps :current="step" size="small">
        <n-step :title="$t('setup_database')" />
        <n-step :title="$t('setup_domain')" />
        <n-step :title="$t('setup_admin')" />
      </n-steps>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-form class="setup-form">
        <template v-if="step === 1">
          <n-form-item :label="$t('setup_dbType')"
            ><n-select v-model:value="form.db_type" :options="dbOptions"
          /></n-form-item>
          <n-form-item :label="$t('formatDsn')"
            ><n-input v-model:value="form.db_dsn"
          /></n-form-item>
        </template>
        <template v-else-if="step === 2">
          <n-form-item :label="$t('setup_domain')"
            ><n-input v-model:value="form.domain" placeholder="example.com"
          /></n-form-item>
        </template>
        <template v-else>
          <n-form-item :label="$t('setup_account')"
            ><n-input v-model:value="form.account"
          /></n-form-item>
          <n-form-item :label="$t('setup_password')"
            ><n-input v-model:value="form.password" type="password" show-password-on="click"
          /></n-form-item>
        </template>
      </n-form>
      <div class="setup-actions">
        <n-button v-if="step > 1" @click="step--">{{ $t('common_backStep') }}</n-button>
        <n-button v-if="step < 3" type="primary" @click="step++">{{ $t('common_next') }}</n-button>
        <n-button v-else type="primary" :loading="loading" @click="save">{{
          $t('setup_finish')
        }}</n-button>
      </div>
    </n-card>
  </main>
</template>
