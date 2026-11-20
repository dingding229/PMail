import { createRouter, createWebHashHistory } from 'vue-router'
const routes = [
  { path: '/', redirect: '/mail' },
  { path: '/mail', name: 'mail', component: () => import('../views/MailView.vue') },
  { path: '/compose', name: 'compose', component: () => import('../views/ComposeView.vue') },
  {
    path: '/settings/:section?',
    name: 'settings',
    component: () => import('../views/SettingsView.vue')
  },
  { path: '/admin/:section?', name: 'admin', component: () => import('../views/AdminView.vue') },
  { path: '/login', name: 'login', component: () => import('../views/LoginView.vue') },
  { path: '/setup', name: 'setup', component: () => import('../views/SetupView.vue') }
]
export const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes
})
