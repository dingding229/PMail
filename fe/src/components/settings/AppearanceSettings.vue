<script setup>
import { useUiStore } from '@/stores/ui'
import { setLocale, i18n } from '@/i18n/i18n'
import { NCard, NForm, NFormItem, NSelect, NSwitch } from 'naive-ui'
const ui = useUiStore()
const locale = i18n.global.locale
</script>
<template>
  <n-card :title="$t('legacy_5c1a574bc2')" class="settings-card"
    ><n-form label-placement="top" label-width="100"
      ><n-form-item :label="$t('legacy_cd99b225a0')"
        ><n-select
          :value="locale"
          :options="[
            { label: $t('legacy_3f1b8e5c2a'), value: 'zh' },
            { label: $t('languageEnglish'), value: 'en' }
          ]"
          @update:value="setLocale" /></n-form-item
      ><n-form-item :label="$t('legacy_192f2990a6')"
        ><n-switch v-model:value="ui.dark" @update:value="ui.toggleDark()" /></n-form-item></n-form
  ></n-card>
</template>
