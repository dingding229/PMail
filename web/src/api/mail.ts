import { apiGet, apiPost, apiUpload } from './client'
import type {
  AddressCounts,
  AttachmentPreviewResponse,
  DraftSaveRequest,
  DraftSaveResponse,
  EmailDetail,
  EmailListRequest,
  EmailListResponse,
  ForwardEmailRequest,
  InlineImageUploadResult,
  SendEmailRequest,
  SendEmailResponse,
} from './types'

const toIds = (value: number | number[]) => (Array.isArray(value) ? value : [value])

export const mailApi = {
  list: (payload: EmailListRequest, signal?: AbortSignal) =>
    apiPost<EmailListResponse>('/api/email/list', payload, { signal }),

  addressCounts: (signal?: AbortSignal) => apiGet<AddressCounts>('/api/email/address_counts', { cache: 'no-store', signal }),

  detail: (id: number, signal?: AbortSignal) =>
    apiPost<EmailDetail>('/api/email/detail', { id }, { signal }),

  markRead: (ids: number | number[], isRead = true) =>
    apiPost<string>('/api/email/read', { ids: toIds(ids), is_read: isRead }),

  star: (ids: number | number[], starred: boolean) =>
    apiPost<string>('/api/email/star', { ids: toIds(ids), starred }),

  remove: (ids: number | number[], forcedDel = false) =>
    apiPost<string>('/api/email/del', { ids: toIds(ids), forcedDel }),

  move: (ids: number | number[], groupId: number, groupName?: string) =>
    apiPost<string>('/api/email/move', {
      ids: toIds(ids),
      group_id: groupId,
      ...(groupName ? { group_name: groupName } : {}),
    }),

  markSpam: (ids: number | number[], isSpam: boolean) =>
    apiPost<string>('/api/email/spam', {
      ids: toIds(ids),
      is_spam: isSpam,
    }),

  send: (payload: SendEmailRequest) =>
    apiPost<SendEmailResponse>('/api/email/send', payload),

  forward: (payload: ForwardEmailRequest) =>
    apiPost<SendEmailResponse>('/api/email/forward', payload),

  saveDraft: async (payload: DraftSaveRequest): Promise<number> => {
    const res = await apiPost<DraftSaveResponse | number>('/api/email/draft/save', payload)
    return typeof res === 'number' ? res : res.id
  },

  deleteDraft: (ids: number | number[]) =>
    apiPost<string>('/api/email/draft/delete', {
      id: Array.isArray(ids) ? ids[0] : ids,
      ids: toIds(ids),
    }),

  /** OpenAPI attachment routes: /api/attachments/download/{emailId}.{index} */
  attachmentDownloadUrl: (emailId: number, index: number, confirmRisk = false) =>
    `/api/attachments/download/${emailId}.${index}${confirmRisk ? '?confirm_risk=1' : ''}`,

  /** OpenAPI inline attachment route: /api/attachments/{emailId}.{index} */
  attachmentInlineUrl: (emailId: number, index: number) =>
    `/api/attachments/${emailId}.${index}`,

  /** OpenAPI text/code attachment preview route: /api/attachments/preview/{emailId}.{index} */
  previewAttachment: (emailId: number, index: number) =>
    apiGet<AttachmentPreviewResponse>(`/api/attachments/preview/${emailId}.${index}`),

  /** C9 inline image upload (multipart field: file, max 10 MiB, png/jpeg/gif/webp) */
  uploadInlineImage: (file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    return apiUpload<InlineImageUploadResult>('/api/compose/inline-image', formData)
  },
}
