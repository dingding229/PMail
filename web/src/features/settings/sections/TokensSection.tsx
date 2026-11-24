import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { toast } from 'sonner'
import { AlertTriangle, Check, Copy, KeyRound, Loader2, Plus, ShieldAlert, Trash2 } from 'lucide-react'
import {
  tokenApi,
  type ApiTokenItem,
  type ApiTokenScope,
  type CreateTokenScope,
} from '@/api'
import { formatMailFullTime, pickDate } from '@/lib/date'
import { queryKeys, useUserMeQuery } from '@/hooks/useMailQueries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  EmptyState,
  SectionCard,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

type ExpiryChoice = 'never' | '30d' | '90d' | '1y'

function getScopeBadgeVariant(
  scope: ApiTokenScope | undefined,
  legacyScope: boolean | undefined,
): 'default' | 'accent' | 'warning' | 'danger' {
  if (legacyScope || scope === 'legacy') return 'danger'
  if (scope === 'admin') return 'warning'
  if (scope === 'send') return 'accent'
  return 'default'
}

export function TokensSection() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const { data: me } = useUserMeQuery(true)
  const isAdmin = me?.is_admin === 1

  const { data: tokens = [], isLoading } = useQuery({
    queryKey: queryKeys.tokens,
    queryFn: () => tokenApi.list(),
  })

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState('')
  const [scope, setScope] = useState<CreateTokenScope>('read')
  const [expiry, setExpiry] = useState<ExpiryChoice>('90d')
  const [revealedToken, setRevealedToken] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [revoking, setRevoking] = useState<ApiTokenItem | null>(null)

  const openCreateDialog = () => {
    setName('')
    setScope('read')
    setExpiry('90d')
    setCreateOpen(true)
  }

  const createMutation = useMutation({
    mutationFn: () => {
      const tokenName = name.trim() || 'API Token'
      const effectiveScope: CreateTokenScope = !isAdmin && scope === 'admin' ? 'read' : scope

      if (expiry === 'never') {
        return tokenApi.create({
          name: tokenName,
          scope: effectiveScope,
          never_expires: true,
        })
      }

      let expiresAt: string
      if (expiry === '30d') expiresAt = dayjs().add(30, 'day').toISOString()
      else if (expiry === '1y') expiresAt = dayjs().add(365, 'day').toISOString()
      else expiresAt = dayjs().add(90, 'day').toISOString()

      return tokenApi.create({
        name: tokenName,
        scope: effectiveScope,
        never_expires: false,
        expires_at: expiresAt,
      })
    },
    onSuccess: (res) => {
      setCreateOpen(false)
      setName('')
      setScope('read')
      setExpiry('90d')
      setRevealedToken(res.token)
      setCopied(false)
      void queryClient.invalidateQueries({ queryKey: queryKeys.tokens })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const revokeMutation = useMutation({
    mutationFn: (id: number) => tokenApi.remove(id),
    onSuccess: () => {
      toast.success(t('settings.tokens.revoked'))
      setRevoking(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.tokens })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const handleCopyToken = async () => {
    if (!revealedToken) return
    try {
      await navigator.clipboard.writeText(revealedToken)
      setCopied(true)
      toast.success(t('common.copied'))
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  return (
    <SectionCard>
      <SectionHeader
        icon={KeyRound}
        title={t('settings.tokens.title')}
        description={t('settings.tokens.desc')}
        action={
          <Button size="sm" onClick={openCreateDialog}>
            <Plus className="h-3.5 w-3.5" />
            {t('settings.tokens.createButton')}
          </Button>
        }
      />

      {isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-16" />
      ) : tokens.length === 0 ? (
        <EmptyState
          icon={KeyRound}
          title={t('settings.tokens.empty')}
          description={t('settings.tokens.emptyDesc')}
          action={
            <Button size="sm" onClick={openCreateDialog}>
              <Plus className="h-3.5 w-3.5" />
              {t('settings.tokens.createButton')}
            </Button>
          }
        />
      ) : (
        <div className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]">
          {tokens.map((tok) => {
            const isLegacy = Boolean(tok.legacy_scope) || tok.scope === 'legacy'
            const scopeKey = isLegacy ? 'legacy' : (tok.scope ?? 'read')
            const scopeLabel = t(`settings.tokens.scopes.${scopeKey}`, { defaultValue: scopeKey })
            const expiresDate = tok.never_expires
              ? null
              : pickDate(tok.expires_at_iso, tok.expires_at)

            return (
              <div
                key={tok.id}
                className="flex flex-col gap-2.5 px-4 py-3"
              >
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-[13.5px] font-semibold text-[var(--ink-primary)]">
                        {tok.name}
                      </span>
                      <code className="rounded bg-[var(--surface-subtle)] px-2 py-0.5 font-mono text-[11.5px] text-[var(--ink-secondary)]">
                        {tok.prefix}••••
                      </code>
                      <Badge variant={getScopeBadgeVariant(tok.scope, tok.legacy_scope)}>
                        {scopeLabel}
                      </Badge>
                    </div>
                    <div className="flex flex-wrap gap-x-4 gap-y-0.5 text-[11.5px] text-[var(--ink-muted)]">
                      <span>
                        {t('settings.tokens.createdAt')}：
                        {formatMailFullTime(pickDate(tok.created_iso, tok.created), i18n.language)}
                      </span>
                      <span>
                        {t('settings.tokens.lastUsedAt')}：
                        {pickDate(tok.last_used_at_iso, tok.last_used_at)
                          ? formatMailFullTime(
                              pickDate(tok.last_used_at_iso, tok.last_used_at),
                              i18n.language,
                            )
                          : t('common.never')}
                      </span>
                      <span>
                        {t('settings.tokens.expiresAt')}：
                        {expiresDate
                          ? formatMailFullTime(expiresDate, i18n.language)
                          : t('settings.tokens.expiresNever')}
                      </span>
                    </div>
                  </div>

                  <Button
                    variant="dangerGhost"
                    size="xs"
                    onClick={() => setRevoking(tok)}
                    className="self-end sm:self-center"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {t('settings.tokens.revoke')}
                  </Button>
                </div>

                {isLegacy && (
                  <div className="flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] px-3 py-1.5 text-[12px] font-medium text-[var(--warning)]">
                    <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
                    <span>{t('settings.tokens.legacyWarning')}</span>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Create Token Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('settings.tokens.createButton')}</DialogTitle>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              createMutation.mutate()
            }}
            className="mt-3 space-y-4"
          >
            <div className="space-y-1.5">
              <Label htmlFor="token_name">{t('settings.tokens.nameLabel')}</Label>
              <Input
                id="token_name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('settings.tokens.namePlaceholder')}
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label>{t('settings.tokens.scopeLabel')}</Label>
              <Select value={scope} onValueChange={(v) => setScope(v as CreateTokenScope)}>
                <SelectTrigger className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="read">{t('settings.tokens.scopes.read')}</SelectItem>
                  <SelectItem value="send">{t('settings.tokens.scopes.send')}</SelectItem>
                  {isAdmin && (
                    <SelectItem value="admin">{t('settings.tokens.scopes.admin')}</SelectItem>
                  )}
                </SelectContent>
              </Select>
              <p className="text-[11.5px] leading-relaxed text-[var(--ink-muted)]">
                {t(`settings.tokens.scopeDescs.${scope}`)}
              </p>
            </div>

            <div className="space-y-1.5">
              <Label>{t('settings.tokens.expiresLabel')}</Label>
              <Select value={expiry} onValueChange={(v) => setExpiry(v as ExpiryChoice)}>
                <SelectTrigger className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="30d">{t('settings.tokens.expires30d')}</SelectItem>
                  <SelectItem value="90d">{t('settings.tokens.expires90d')}</SelectItem>
                  <SelectItem value="1y">{t('settings.tokens.expires1y')}</SelectItem>
                  <SelectItem value="never">{t('settings.tokens.expiresNever')}</SelectItem>
                </SelectContent>
              </Select>
              {expiry === 'never' && (
                <div className="mt-2 flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] px-3 py-2 text-[12px] leading-relaxed text-[var(--warning)]">
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span>{t('settings.tokens.expiresNeverWarning')}</span>
                </div>
              )}
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" size="sm" onClick={() => setCreateOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={createMutation.isPending}>
                {createMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.create')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* One-Time Plaintext Token Reveal Dialog */}
      <Dialog
        open={revealedToken !== null}
        onOpenChange={(open) => !open && setRevealedToken(null)}
      >
        <DialogContent className="max-w-md" hideClose>
          <DialogHeader>
            <DialogTitle>{t('settings.tokens.revealTitle')}</DialogTitle>
          </DialogHeader>

          <div className="mt-3 space-y-3">
            <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--warning)]/30 bg-[var(--warning-soft)] px-3 py-2 text-[12.5px] text-[var(--warning)]">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t('settings.tokens.revealWarning')}</span>
            </div>

            <div className="flex items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-2.5">
              <code className="min-w-0 flex-1 break-all font-mono text-[12.5px] text-[var(--ink-primary)] select-all">
                {revealedToken}
              </code>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => void handleCopyToken()}
              >
                {copied ? (
                  <Check className="h-3.5 w-3.5 text-[var(--success)]" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
                {t('common.copy')}
              </Button>
            </div>
          </div>

          <DialogFooter>
            <Button variant="primary" size="sm" onClick={() => setRevealedToken(null)}>
              {t('settings.tokens.revealDone')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Revoke Confirm */}
      <Dialog open={revoking !== null} onOpenChange={(open) => !open && setRevoking(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.tokens.revokeTitle')}</DialogTitle>
            <DialogDescription>
              {revoking ? t('settings.tokens.revokeConfirm', { name: revoking.name }) : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setRevoking(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={revokeMutation.isPending}
              onClick={() => revoking && revokeMutation.mutate(revoking.id)}
            >
              {t('settings.tokens.revoke')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
