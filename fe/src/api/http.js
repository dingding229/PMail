import { http } from '@/utils/axios'
export const post = (url, data = {}) => http.post(url, data)
export const get = (url, params = {}) => http.get(url, { params })
