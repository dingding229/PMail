import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import zhCN from './locales/zh-CN'
import { getRawStorage, setRawStorage } from '@/lib/storage'
import { setDayjsLocale } from '@/lib/date'

export type SupportedLanguage = 'zh-CN' | 'en'

export function normalizeLanguage(raw?: string | null): SupportedLanguage {
  if (!raw) return 'zh-CN'
  return raw.toLowerCase().startsWith('en') ? 'en' : 'zh-CN'
}

const initialLang = normalizeLanguage(getRawStorage('lang') || 'zh-CN')

void i18n.use(initReactI18next).init({
  resources: {
    'zh-CN': { translation: zhCN },
  },
  lng: 'zh-CN',
  fallbackLng: 'zh-CN',
  interpolation: {
    escapeValue: false,
  },
})

async function ensureEnglishLoaded() {
  if (i18n.hasResourceBundle('en', 'translation')) return
  const mod = await import('./locales/en')
  i18n.addResourceBundle('en', 'translation', mod.default, true, true)
}

if (initialLang === 'en') {
  void ensureEnglishLoaded().then(() => {
    void i18n.changeLanguage('en')
    setDayjsLocale('en')
    if (typeof document !== 'undefined') document.documentElement.lang = 'en'
  })
} else {
  setDayjsLocale('zh-CN')
  if (typeof document !== 'undefined') document.documentElement.lang = 'zh-CN'
}

export function setAppLanguage(lang: SupportedLanguage, persist = true): void {
  const normalized = normalizeLanguage(lang)
  if (persist) {
    setRawStorage('lang', normalized)
  }
  const apply = () => {
    void i18n.changeLanguage(normalized)
    setDayjsLocale(normalized)
    if (typeof document !== 'undefined') {
      document.documentElement.lang = normalized
    }
  }
  if (normalized === 'en') {
    void ensureEnglishLoaded().then(apply)
  } else {
    apply()
  }
}

export function getAppLanguage(): SupportedLanguage {
  return normalizeLanguage(i18n.language)
}

export default i18n
