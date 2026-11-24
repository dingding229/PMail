import type { ComponentPropsWithoutRef, ReactNode } from 'react'
import { cn } from '@/lib/cn'

export function DataTable({
  className,
  children,
}: {
  className?: string
  children: ReactNode
}) {
  return (
    <div
      className={cn(
        'overflow-x-auto rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]',
        className,
      )}
    >
      <table className="w-full text-left text-[13px]">{children}</table>
    </div>
  )
}

export function DataTableHead({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<'thead'>) {
  return (
    <thead
      className={cn(
        'border-b border-[var(--border-hairline)] bg-[var(--surface-subtle)]/70 text-[12px] font-medium text-[var(--ink-muted)]',
        className,
      )}
      {...props}
    >
      {children}
    </thead>
  )
}

export function DataTableHeaderCell({
  className,
  align = 'left',
  children,
  ...props
}: ComponentPropsWithoutRef<'th'> & { align?: 'left' | 'right' | 'center' }) {
  return (
    <th
      className={cn(
        'px-4 py-2.5 font-medium',
        align === 'right' && 'text-right',
        align === 'center' && 'text-center',
        className,
      )}
      {...props}
    >
      {children}
    </th>
  )
}

export function DataTableBody({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<'tbody'>) {
  return (
    <tbody
      className={cn('divide-y divide-[var(--border-hairline)]', className)}
      {...props}
    >
      {children}
    </tbody>
  )
}

export function DataTableRow({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<'tr'>) {
  return (
    <tr
      className={cn('transition-colors hover:bg-[var(--surface-hover)]/50', className)}
      {...props}
    >
      {children}
    </tr>
  )
}

export function DataTableCell({
  className,
  align = 'left',
  children,
  ...props
}: ComponentPropsWithoutRef<'td'> & { align?: 'left' | 'right' | 'center' }) {
  return (
    <td
      className={cn(
        'px-4 py-3',
        align === 'right' && 'text-right',
        align === 'center' && 'text-center',
        className,
      )}
      {...props}
    >
      {children}
    </td>
  )
}
