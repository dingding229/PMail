import * as React from 'react'
import { cn } from '@/lib/cn'

export const Label = React.forwardRef<
  HTMLLabelElement,
  React.LabelHTMLAttributes<HTMLLabelElement>
>(({ className, ...props }, ref) => (
  <label
    ref={ref}
    className={cn(
      'text-[13px] font-medium leading-none text-[var(--ink-primary)] peer-disabled:cursor-not-allowed peer-disabled:opacity-70',
      className,
    )}
    {...props}
  />
))
Label.displayName = 'Label'
