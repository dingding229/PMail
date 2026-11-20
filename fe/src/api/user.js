import { post } from './http'
export const userApi = {
  list: () => post('/api/user/list'),
  me: (data) => post('/api/user/me', data || {}),
  create: (data) => post('/api/user/create', data),
  update: (data) => post('/api/user/edit', data),
  changePassword: (data) => post('/api/user/change_password', data),
  resetPassword: (data) => post('/api/user/reset_password', data),
  quota: (data) => post('/api/user/set_quota', data),
  remove: (id) => post('/api/user/delete', { id })
}
