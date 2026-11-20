import { createI18n } from 'vue-i18n'
import zh from './zh'
import en from './en'

const browser = navigator.language?.toLowerCase().startsWith('zh') ? 'zh' : 'en'

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: localStorage.getItem('pmail-locale') || browser,
  fallbackLocale: 'en',
  messages: { zh, en }
})

export const setLocale = (value) => {
  i18n.global.locale.value = value
  localStorage.setItem('pmail-locale', value)
}
