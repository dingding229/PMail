import { post } from './http'

const ids = (value) => (Array.isArray(value) ? value : [value])

export const mailApi = {
  list: (data) => post('/api/email/list', data),
  detail: (id) => post('/api/email/detail', { id }),
  read: (value) => post('/api/email/read', { ids: ids(value) }),
  remove: (value) => post('/api/email/del', { ids: ids(value) }),
  move: (value, groupId) => post('/api/email/move', { ids: ids(value), group_id: groupId }),
  send: (data) => post('/api/email/send', data),
  forward: (data) => post('/api/email/forward', data)
}
