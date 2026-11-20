import { post } from './http'
export const addressApi = {
  list: (userId) => post('/api/address/list', userId ? { user_id: userId } : {}),
  create: (data) => post('/api/address/create', data),
  update: (data) => post('/api/address/update', data),
  remove: (id) => post('/api/address/delete', { id }),
  primary: (id) => post('/api/address/set_primary', { id })
}
