import { post } from './http'
export const domainApi = {
  list: () => post('/api/domain/list'),
  create: (data) => post('/api/domain/create', data),
  remove: (id) => post('/api/domain/delete', { id }),
  primary: (id) => post('/api/domain/set_default', { id }),
  dns: (id) => post('/api/domain/dns', { id })
}
