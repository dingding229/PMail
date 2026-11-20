import { post } from './http'
export const setupApi = {
  save: (data) => post('/api/setup', data),
  get: (data) => post('/api/setup', data)
}
