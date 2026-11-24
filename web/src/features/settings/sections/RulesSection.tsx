import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  ArrowDown,
  ArrowUp,
  Edit3,
  Filter,
  GripVertical,
  Loader2,
  Plus,
  Trash2,
  X,
} from 'lucide-react'
import {
  ruleApi,
  SYSTEM_GROUP_IDS,
  type MailRule,
  type RuleActionCode,
  type RuleCondition,
  type RuleConditionField,
  type RuleConditionType,
} from '@/api'
import { queryKeys, useGroupOptionsQuery } from '@/hooks/useMailQueries'
import { cn } from '@/lib/cn'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
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

const FIELDS: RuleConditionField[] = ['From', 'Subject', 'To', 'Cc', 'Text', 'Html', 'Content']
const OPERATORS: RuleConditionType[] = ['contains', 'equal', 'regex']

export function RulesSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: rules = [], isLoading } = useQuery({
    queryKey: queryKeys.rules,
    queryFn: () => ruleApi.list(),
  })
  const { data: groupOptions = [] } = useGroupOptionsQuery(true)

  const movableGroups = useMemo(
    () =>
      groupOptions.filter(
        (g) => g.id < SYSTEM_GROUP_IDS.INBOX || g.id === SYSTEM_GROUP_IDS.JUNK,
      ),
    [groupOptions],
  )

  const [modalOpen, setModalOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<MailRule | null>(null)
  const [name, setName] = useState('')
  const [conditions, setConditions] = useState<RuleCondition[]>([])
  const [action, setAction] = useState<RuleActionCode>(1)
  const [params, setParams] = useState('')
  const [formError, setFormError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<MailRule | null>(null)
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null)

  const sortedRules = useMemo(
    () => [...rules].sort((a, b) => b.sort - a.sort || a.id - b.id),
    [rules],
  )

  const saveMutation = useMutation({
    mutationFn: async () => {
      const topSort = sortedRules[0]?.sort ?? 10
      const payload = {
        id: editingRule?.id ?? 0,
        name: name.trim(),
        rules: conditions.filter((c) => c.rule.trim().length > 0),
        action,
        params: action === 2 || action === 4 ? params.trim() : '',
        sort: editingRule ? editingRule.sort : topSort + 10,
      }
      return editingRule ? ruleApi.update(payload) : ruleApi.add(payload)
    },
    onSuccess: () => {
      toast.success(t('settings.rules.saved'))
      setModalOpen(false)
      void queryClient.invalidateQueries({ queryKey: queryKeys.rules })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (rule: MailRule) => ruleApi.remove(rule.id),
    onSuccess: (_res, rule) => {
      toast.success(t('settings.rules.deleted', { name: rule.name }))
      setDeleting(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.rules })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const reorderMutation = useMutation({
    mutationFn: async (reordered: MailRule[]) => {
      const total = reordered.length
      await Promise.all(
        reordered.map((r, idx) =>
          ruleApi.update({
            id: r.id,
            name: r.name,
            rules: r.rules ?? [],
            action: r.action,
            params: r.params,
            sort: (total - idx) * 10,
          }),
        ),
      )
    },
    onSuccess: () => {
      toast.success(t('settings.rules.sortSaved'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.rules })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const openCreate = () => {
    setEditingRule(null)
    setName('')
    setConditions([{ field: 'From', type: 'contains', rule: '' }])
    setAction(1)
    setParams('')
    setFormError(null)
    setModalOpen(true)
  }

  const openEdit = (rule: MailRule) => {
    setEditingRule(rule)
    setName(rule.name)
    setConditions(
      rule.rules && rule.rules.length > 0
        ? rule.rules.map((c) => ({ ...c }))
        : [{ field: 'From', type: 'contains', rule: '' }],
    )
    setAction(rule.action)
    setParams(rule.params || '')
    setFormError(null)
    setModalOpen(true)
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setFormError(null)
    if (!name.trim()) {
      setFormError(t('settings.rules.validation.nameRequired'))
      return
    }
    if (action === 2 && !params.trim()) {
      setFormError(t('settings.rules.validation.forwardRequired'))
      return
    }
    if (action === 4 && !params.trim()) {
      setFormError(t('settings.rules.validation.groupRequired'))
      return
    }
    saveMutation.mutate()
  }

  const moveItem = (fromIdx: number, toIdx: number) => {
    if (fromIdx === toIdx || toIdx < 0 || toIdx >= sortedRules.length) return
    const next = [...sortedRules]
    const [moved] = next.splice(fromIdx, 1)
    if (!moved) return
    next.splice(toIdx, 0, moved)
    reorderMutation.mutate(next)
  }

  const describeAction = (rule: MailRule) => {
    const base = t(`settings.rules.actions.${rule.action}` as const)
    if (rule.action === 2 && rule.params) return `${base}：${rule.params}`
    if (rule.action === 4 && rule.params) {
      const grp = groupOptions.find((g) => String(g.id) === rule.params)
      return `${base}：${grp?.name ?? rule.params}`
    }
    return base
  }

  return (
    <SectionCard>
      <SectionHeader
        icon={Filter}
        title={t('settings.rules.title')}
        description={t('settings.rules.desc')}
        action={
          <Button size="sm" onClick={openCreate}>
            <Plus className="h-3.5 w-3.5" />
            {t('settings.rules.addButton')}
          </Button>
        }
      />

      {isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-20" />
      ) : sortedRules.length === 0 ? (
        <EmptyState
          icon={Filter}
          title={t('settings.rules.empty')}
          description={t('settings.rules.emptyDesc')}
          action={
            <Button size="sm" onClick={openCreate}>
              <Plus className="h-3.5 w-3.5" />
              {t('settings.rules.addButton')}
            </Button>
          }
        />
      ) : (
        <div className="space-y-2">
          {sortedRules.map((rule, idx) => (
            <div
              key={rule.id}
              draggable
              onDragStart={() => setDragIndex(idx)}
              onDragOver={(e) => {
                e.preventDefault()
                setDragOverIndex(idx)
              }}
              onDragLeave={() => setDragOverIndex(null)}
              onDrop={(e) => {
                e.preventDefault()
                if (dragIndex !== null) moveItem(dragIndex, idx)
                setDragIndex(null)
                setDragOverIndex(null)
              }}
              onDragEnd={() => {
                setDragIndex(null)
                setDragOverIndex(null)
              }}
              className={cn(
                'flex flex-col gap-3 rounded-[var(--radius-md)] border bg-[var(--surface-canvas)] p-3.5 transition-colors sm:flex-row sm:items-center sm:justify-between',
                dragOverIndex === idx
                  ? 'border-[var(--accent)] bg-[var(--accent-soft)]/30'
                  : 'border-[var(--border-hairline)]',
              )}
            >
              <div className="flex min-w-0 items-start gap-2.5">
                <div
                  title={t('settings.rules.desc')}
                  className="mt-0.5 cursor-grab text-[var(--ink-muted)] hover:text-[var(--ink-primary)] active:cursor-grabbing"
                >
                  <GripVertical className="h-4 w-4" />
                </div>
                <div className="min-w-0 flex-1 space-y-1.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-[13.5px] font-semibold text-[var(--ink-primary)]">
                      {rule.name}
                    </span>
                    <Badge variant="accent">{describeAction(rule)}</Badge>
                  </div>
                  <div className="flex flex-wrap items-center gap-1.5 text-[12px] text-[var(--ink-secondary)]">
                    {!rule.rules || rule.rules.length === 0 ? (
                      <span className="text-[var(--ink-muted)]">{t('settings.rules.matchAll')}</span>
                    ) : (
                      rule.rules.map((c, cIdx) => (
                        <span key={cIdx} className="inline-flex items-center gap-1">
                          {cIdx > 0 && (
                            <span className="font-medium text-[var(--ink-muted)]">
                              {t('settings.rules.andJoin')}
                            </span>
                          )}
                          <span className="rounded bg-[var(--surface-subtle)] px-2 py-0.5 font-mono text-[11.5px]">
                            {t(`settings.rules.fields.${c.field}` as const)}{' '}
                            {t(`settings.rules.operators.${c.type}` as const)} “{c.rule}”
                          </span>
                        </span>
                      ))
                    )}
                  </div>
                </div>
              </div>

              <div className="flex shrink-0 items-center gap-1 self-end sm:self-center">
                <Button
                  variant="ghost"
                  size="iconSm"
                  disabled={idx === 0 || reorderMutation.isPending}
                  onClick={() => moveItem(idx, idx - 1)}
                  aria-label={t('settings.rules.moveUp')}
                  title={t('settings.rules.moveUp')}
                >
                  <ArrowUp className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="iconSm"
                  disabled={idx === sortedRules.length - 1 || reorderMutation.isPending}
                  onClick={() => moveItem(idx, idx + 1)}
                  aria-label={t('settings.rules.moveDown')}
                  title={t('settings.rules.moveDown')}
                >
                  <ArrowDown className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="iconSm"
                  onClick={() => openEdit(rule)}
                  aria-label={t('common.edit')}
                >
                  <Edit3 className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="dangerGhost"
                  size="iconSm"
                  onClick={() => setDeleting(rule)}
                  aria-label={t('common.delete')}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Rule Builder Modal */}
      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent className="max-w-xl max-h-[88vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {editingRule ? t('settings.rules.modalEdit') : t('settings.rules.modalAdd')}
            </DialogTitle>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="mt-3 space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="rule_name">{t('settings.rules.nameLabel')}</Label>
              <Input
                id="rule_name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('settings.rules.namePlaceholder')}
                autoFocus
              />
            </div>

            {/* Condition Builder */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>{t('settings.rules.conditionsTitle')}</Label>
                <Button
                  type="button"
                  variant="outline"
                  size="xs"
                  onClick={() =>
                    setConditions((prev) => [
                      ...prev,
                      { field: 'Subject', type: 'contains', rule: '' },
                    ])
                  }
                >
                  <Plus className="h-3 w-3" />
                  {t('settings.rules.addCondition')}
                </Button>
              </div>

              <div className="space-y-2">
                {conditions.map((cond, idx) => (
                  <div
                    key={idx}
                    className="grid grid-cols-1 gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 p-2.5 sm:grid-cols-[120px_115px_1fr_auto]"
                  >
                    <Select
                      value={cond.field}
                      onValueChange={(v) =>
                        setConditions((prev) =>
                          prev.map((c, i) =>
                            i === idx ? { ...c, field: v as RuleConditionField } : c,
                          ),
                        )
                      }
                    >
                      <SelectTrigger className="h-8">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {FIELDS.map((f) => (
                          <SelectItem key={f} value={f}>
                            {t(`settings.rules.fields.${f}` as const)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>

                    <Select
                      value={cond.type}
                      onValueChange={(v) =>
                        setConditions((prev) =>
                          prev.map((c, i) =>
                            i === idx ? { ...c, type: v as RuleConditionType } : c,
                          ),
                        )
                      }
                    >
                      <SelectTrigger className="h-8">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {OPERATORS.map((op) => (
                          <SelectItem key={op} value={op}>
                            {t(`settings.rules.operators.${op}` as const)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>

                    <Input
                      value={cond.rule}
                      onChange={(e) =>
                        setConditions((prev) =>
                          prev.map((c, i) => (i === idx ? { ...c, rule: e.target.value } : c)),
                        )
                      }
                      placeholder={t('settings.rules.patternPlaceholder')}
                      className="h-8"
                    />

                    <Button
                      type="button"
                      variant="ghost"
                      size="iconSm"
                      onClick={() => setConditions((prev) => prev.filter((_, i) => i !== idx))}
                      aria-label={t('common.delete')}
                    >
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                ))}
              </div>
            </div>

            {/* Action Selector */}
            <div className="space-y-1.5">
              <Label>{t('settings.rules.actionLabel')}</Label>
              <Select
                value={String(action)}
                onValueChange={(v) => {
                  const nextAction = Number(v) as RuleActionCode
                  setAction(nextAction)
                  if (nextAction === 4 && movableGroups[0]) {
                    setParams(String(movableGroups[0].id))
                  } else if (nextAction !== 2) {
                    setParams('')
                  }
                }}
              >
                <SelectTrigger className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {([1, 2, 3, 4] as RuleActionCode[]).map((code) => (
                    <SelectItem key={code} value={String(code)}>
                      {t(`settings.rules.actions.${code}` as const)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {action === 2 && (
              <div className="space-y-1.5">
                <Label htmlFor="rule_forward_to">{t('settings.rules.forwardToLabel')}</Label>
                <Input
                  id="rule_forward_to"
                  value={params}
                  onChange={(e) => setParams(e.target.value)}
                  placeholder={t('settings.rules.forwardToPlaceholder')}
                />
              </div>
            )}

            {action === 4 && (
              <div className="space-y-1.5">
                <Label>{t('settings.rules.targetGroupLabel')}</Label>
                <Select value={params} onValueChange={setParams}>
                  <SelectTrigger className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {movableGroups.map((g) => (
                      <SelectItem key={g.id} value={String(g.id)}>
                        {g.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}

            {formError && <p className="text-[12.5px] text-[var(--danger)]">{formError}</p>}

            <DialogFooter>
              <Button type="button" variant="outline" size="sm" onClick={() => setModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={saveMutation.isPending}>
                {saveMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.rules.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deleting ? t('settings.rules.deleteConfirm', { name: deleting.name }) : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setDeleting(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={deleteMutation.isPending}
              onClick={() => deleting && deleteMutation.mutate(deleting)}
            >
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
