import { defineStore } from 'pinia'
import { api, unwrap } from '@/api'
import { router } from '@/router'
export const useAuthStore = defineStore('auth', {
  state: () => ({ user: null, ready: false }),
  getters: { isAdmin: (s) => s.user?.is_admin === 1 || s.user?.isAdmin === 1 },
  actions: {
    async load() {
      try {
        const res = await api.users.me()
        if (res.errorNo === 0) this.user = unwrap(res)
      } finally {
        this.ready = true
      }
    },
    async login(account, password) {
      const res = await api.login({ account, password })
      if (res.errorNo) throw new Error(res.errorMsg || 'Login failed')
      await this.load()
      await router.replace('/mail')
      return res
    },
    async logout() {
      await api.logout()
      this.user = null
      await router.replace('/login')
    }
  }
})
