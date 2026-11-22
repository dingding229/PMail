export { post, get } from './http'
export { authApi } from './auth'
export { mailApi } from './mail'
export { addressApi } from './address'
export { userApi } from './user'
export { domainApi } from './domain'
export {
  groupApi,
  forwardApi,
  ruleApi,
  tokenApi,
  queueApi,
  externalApi
} from './settings'
export { setupApi } from './setup'
export const unwrap = (response, fallback = null) => response?.data ?? fallback

import { authApi } from './auth'
import { mailApi } from './mail'
import { addressApi } from './address'
import { userApi } from './user'
import { domainApi } from './domain'
import {
  groupApi,
  forwardApi,
  ruleApi,
  tokenApi,
  queueApi,
  externalApi
} from './settings'
import { setupApi } from './setup'
export const api = {
  login: authApi.login,
  logout: authApi.logout,
  users: userApi,
  mail: mailApi,
  addresses: addressApi,
  domains: domainApi,
  groups: groupApi,
  forward: forwardApi,
  rules: ruleApi,
  tokens: tokenApi,
  queue: queueApi,
  external: externalApi,
  setup: setupApi.save
}
