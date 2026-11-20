import { defineStore } from 'pinia'
export const useUiStore = defineStore('ui', {
  state: () => ({
    drawer: false,
    dark:
      localStorage.getItem('pmail-theme') === 'dark' ||
      (localStorage.getItem('pmail-theme') !== 'light' &&
        matchMedia('(prefers-color-scheme: dark)').matches),
    sidebarWidth: Number(localStorage.getItem('pmail-sidebar-width') || 240),
    readerWidth: Number(localStorage.getItem('pmail-reader-width') || 390)
  }),
  actions: {
    toggleDark() {
      this.dark = !this.dark
      localStorage.setItem('pmail-theme', this.dark ? 'dark' : 'light')
    },
    setSidebarWidth(v) {
      this.sidebarWidth = v
      localStorage.setItem('pmail-sidebar-width', v)
    },
    setReaderWidth(v) {
      this.readerWidth = v
      localStorage.setItem('pmail-reader-width', v)
    }
  }
})
