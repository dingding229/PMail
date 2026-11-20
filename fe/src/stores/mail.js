import { defineStore } from 'pinia'
import { api, unwrap } from '@/api'
export const useMailStore = defineStore('mail', {
  state: () => ({
    tag: '{"type":0,"status":-1}',
    keyword: '',
    page: 1,
    pageSize: 20,
    list: [],
    total: 0,
    selected: null,
    loading: false,
    groups: []
  }),
  actions: {
    async load() {
      this.loading = true
      try {
        const res = await api.mail.list({
          tag: this.tag,
          keyword: this.keyword,
          current_page: this.page,
          page_size: this.pageSize
        })
        this.list = unwrap(res, {}).list || []
        this.total = unwrap(res, {}).total_page || 0
      } finally {
        this.loading = false
      }
    },
    async loadGroups() {
      const res = await api.groups.list()
      this.groups = unwrap(res, []) || []
    },
    select(mail) {
      this.selected = mail
    },
    setFolder(tag) {
      this.tag = tag
      this.page = 1
      this.selected = null
      return this.load()
    }
  }
})
