import { post } from './http'
export const authApi = {
  login: (data) => post('/api/login', data),
  logout: () => post('/api/logout')
}
