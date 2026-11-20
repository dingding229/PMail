import axios from 'axios'
import { router } from '@/router'
import { i18n } from '@/i18n/i18n'

const http = axios.create({
  baseURL: import.meta.env.VITE_APP_URL || '',
  timeout: 60000,
  headers: {
    'Content-Type': 'application/json;charset=UTF-8',
    Lang: navigator.language?.startsWith('zh') ? 'zh' : 'en'
  }
})

function friendlyError(error) {
  const status = error?.response?.status
  if (status === 429) return new Error(i18n.global.t('auth_locked'))
  if (status === 403) return new Error(i18n.global.t('common_httpError'))
  if (status >= 500) return new Error(i18n.global.t('common_httpError'))
  return new Error(error?.response?.data?.errorMsg || i18n.global.t('common_httpError'))
}

http.interceptors.response.use(
  async (response) => {
    const data = response.data || {}
    if (data.errorNo === 403)
      await router.replace({
        path: '/login',
        query: { redirect: router.currentRoute.value.fullPath }
      })
    if (data.errorNo === 402)
      await router.replace({
        path: '/setup',
        query: { redirect: router.currentRoute.value.fullPath }
      })
    if (data.errorNo === 429) throw new Error(i18n.global.t('auth_locked'))
    return data
  },
  async (error) => {
    if (error.response?.status === 403)
      await router.replace({
        path: '/login',
        query: { redirect: router.currentRoute.value.fullPath }
      })
    throw friendlyError(error)
  }
)
export { http }
