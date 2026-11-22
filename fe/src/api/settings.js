import { post, get } from './http'
export const groupApi = {
  list: () => post('/api/group/list'),
  add: (data) => post('/api/group/add', data),
  remove: (id) => post('/api/group/del', { id })
}
export const forwardApi = {
  get: () => post('/api/forward/get'),
  set: (data) => post('/api/forward/set', data)
}
export const ruleApi = {
  list: () => post('/api/rule/get'),
  save: (data) => post('/api/rule/add', data),
  remove: (id) => post('/api/rule/del', { id })
}
export const tokenApi = {
  list: () => post('/api/token/list'),
  create: (data) => post('/api/token/create', data),
  remove: (id) => post('/api/token/delete', { id })
}
export const queueApi = {
  list: () => post('/api/queue/list'),
  retry: (id) => post('/api/queue/retry', { id }),
  remove: (id) => post('/api/queue/delete', { id })
}
export const externalApi = {
  list: () => post('/api/external/list'),
  create: (data) => post('/api/external/create', data),
  update: (data) => post('/api/external/update', data),
  remove: (id) => post('/api/external/delete', { id }),
  test: (data) => post('/api/external/test', data),
  sync: (id) => post('/api/external/sync_now', { id }),
  presets: () => get('/api/external/presets')
}
