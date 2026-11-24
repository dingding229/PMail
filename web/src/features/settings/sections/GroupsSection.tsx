import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Folder, Loader2, Plus, Trash2 } from 'lucide-react'
import { SYSTEM_GROUP_IDS, type GroupOption } from '@/api'
import {
  useCreateGroupMutation,
  useDeleteGroupMutation,
  useGroupOptionsQuery,
} from '@/hooks/useMailQueries'
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

export function GroupsSection() {
  const { t } = useTranslation()
  const { data: allGroups = [], isLoading } = useGroupOptionsQuery(true)
  const createGroup = useCreateGroupMutation()
  const deleteGroup = useDeleteGroupMutation()

  const customGroups = useMemo(
    () => allGroups.filter((g) => g.id < SYSTEM_GROUP_IDS.INBOX),
    [allGroups],
  )

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState('')
  const [parentId, setParentId] = useState('0')
  const [deleting, setDeleting] = useState<GroupOption | null>(null)

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) return
    await createGroup.mutateAsync({ name: trimmed, parent_id: Number(parentId) || 0 })
    setName('')
    setParentId('0')
    setCreateOpen(false)
  }

  return (
    <SectionCard>
      <SectionHeader
        icon={Folder}
        title={t('settings.groups.title')}
        description={t('settings.groups.desc')}
        action={
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="h-3.5 w-3.5" />
            {t('settings.groups.addButton')}
          </Button>
        }
      />

      {isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-14" />
      ) : customGroups.length === 0 ? (
        <EmptyState
          icon={Folder}
          title={t('settings.groups.empty')}
          description={t('settings.groups.emptyDesc')}
          action={
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="h-3.5 w-3.5" />
              {t('settings.groups.addButton')}
            </Button>
          }
        />
      ) : (
        <div className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]">
          {customGroups.map((g) => (
            <div
              key={g.id}
              className="flex items-center justify-between gap-3 px-4 py-3"
            >
              <div className="flex min-w-0 items-center gap-3">
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--surface-subtle)] text-[var(--accent)]">
                  <Folder className="h-4 w-4" />
                </div>
                <div className="min-w-0">
                  <p className="truncate text-[13.5px] font-medium text-[var(--ink-primary)]">
                    {g.name}
                  </p>
                  <p className="truncate font-mono text-[11.5px] text-[var(--ink-muted)]">
                    {g.full_path || `/${g.name}`}
                  </p>
                </div>
              </div>
              <Button
                variant="dangerGhost"
                size="iconSm"
                aria-label={t('common.delete')}
                onClick={() => setDeleting(g)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}

      {/* Create Group Modal */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.groups.addButton')}</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleCreate} className="mt-3 space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="group_name">{t('settings.groups.nameLabel')}</Label>
              <Input
                id="group_name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('settings.groups.namePlaceholder')}
                maxLength={10}
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="group_parent">{t('settings.groups.parentLabel')}</Label>
              <Select value={parentId} onValueChange={setParentId}>
                <SelectTrigger id="group_parent" className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">{t('settings.groups.rootOption')}</SelectItem>
                  {customGroups.map((g) => (
                    <SelectItem key={g.id} value={String(g.id)}>
                      {g.full_path || g.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" size="sm" onClick={() => setCreateOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={!name.trim() || createGroup.isPending}>
                {createGroup.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.create')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm Dialog */}
      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('nav.deleteGroup')}</DialogTitle>
            <DialogDescription>
              {deleting ? t('nav.deleteGroupConfirm', { name: deleting.name }) : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setDeleting(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={deleteGroup.isPending}
              onClick={async () => {
                if (!deleting) return
                await deleteGroup.mutateAsync({ id: deleting.id, name: deleting.name })
                setDeleting(null)
              }}
            >
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
