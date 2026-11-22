import * as React from 'react'
import * as SelectPrimitive from '@radix-ui/react-select'
import { Check, ChevronDown } from 'lucide-react'
import { cn } from '@/lib/cn'

export const Select = SelectPrimitive.Root
export const SelectValue = SelectPrimitive.Value

export const SelectTrigger = React.forwardRef<
  React.ComponentRef<typeof SelectPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Trigger>
>(({ className, children, ...props }, ref) => (
  <SelectPrimitive.Trigger
    ref={ref}
    className={cn(
      'flex h-8 w-full items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-2.5 text-[13px] text-[var(--ink-primary)] transition-colors hover:bg-[var(--surface-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-ring)] disabled:cursor-not-allowed disabled:opacity-50',
      className,
    )}
    {...props}
  >
    <span className="truncate">{children}</span>
    <SelectPrimitive.Icon asChild>
      <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--ink-muted)]" />
    </SelectPrimitive.Icon>
  </SelectPrimitive.Trigger>
))
SelectTrigger.displayName = 'SelectTrigger'

export const SelectContent = React.forwardRef<
  React.ComponentRef<typeof SelectPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Content>
>(({ className, children, position = 'popper', ...props }, ref) => (
  <SelectPrimitive.Portal>
    <SelectPrimitive.Content
      ref={ref}
      position={position}
      className={cn(
        'z-50 max-h-64 min-w-[8rem] overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-1 text-[var(--ink-primary)] shadow-[var(--shadow-md)]',
        position === 'popper' && 'translate-y-1',
        className,
      )}
      {...props}
    >
      <SelectPrimitive.Viewport className="p-0.5">{children}</SelectPrimitive.Viewport>
    </SelectPrimitive.Content>
  </SelectPrimitive.Portal>
))
SelectContent.displayName = 'SelectContent'

export const SelectItem = React.forwardRef<
  React.ComponentRef<typeof SelectPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Item>
>(({ className, children, ...props }, ref) => (
  <SelectPrimitive.Item
    ref={ref}
    className={cn(
      'relative flex w-full cursor-pointer select-none items-center rounded-[var(--radius-xs)] py-1.5 pl-7 pr-2.5 text-[13px] outline-none focus:bg-[var(--surface-hover)] data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
      className,
    )}
    {...props}
  >
    <span className="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
      <SelectPrimitive.ItemIndicator>
        <Check className="h-3.5 w-3.5 text-[var(--accent)]" />
      </SelectPrimitive.ItemIndicator>
    </span>
    <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
  </SelectPrimitive.Item>
))
SelectItem.displayName = 'SelectItem'
