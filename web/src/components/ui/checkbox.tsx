import * as React from 'react'
import * as CheckboxPrimitive from '@radix-ui/react-checkbox'
import { Check, Minus } from 'lucide-react'
import { cn } from '@/lib/cn'

export const Checkbox = React.forwardRef<
  React.ComponentRef<typeof CheckboxPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>
>(({ className, ...props }, ref) => (
  <CheckboxPrimitive.Root
    ref={ref}
    className={cn(
      'peer h-4 w-4 shrink-0 rounded-[4px] border border-[var(--border-strong)] bg-[var(--surface-canvas)] transition-colors duration-[var(--duration-fast)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent-ring)] disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-[var(--accent)] data-[state=checked]:bg-[var(--accent)] data-[state=checked]:text-white data-[state=indeterminate]:border-[var(--accent)] data-[state=indeterminate]:bg-[var(--accent)] data-[state=indeterminate]:text-white',
      className,
    )}
    {...props}
  >
    <CheckboxPrimitive.Indicator className="flex items-center justify-center text-current">
      {props.checked === 'indeterminate' ? (
        <Minus className="h-3 w-3 stroke-[2.75]" />
      ) : (
        <Check className="h-3 w-3 stroke-[2.75]" />
      )}
    </CheckboxPrimitive.Indicator>
  </CheckboxPrimitive.Root>
))
Checkbox.displayName = 'Checkbox'
