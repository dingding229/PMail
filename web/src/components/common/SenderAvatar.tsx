import { useContext } from 'react'
import { cn } from '@/lib/cn'
import { getAvatarPalette, getInitials } from '@/lib/format'
import { PreferencesContext } from '@/lib/preferences'
import { matchBrandByEmail } from '@/lib/brandAvatars'

interface SenderAvatarProps {
  name?: string
  email?: string
  size?: 'xs' | 'sm' | 'md' | 'lg'
  /** Must be true ONLY when DMARC auth_status === 'pass' and no medium/high risk */
  verified?: boolean
  /** Optional override (primarily for tests); otherwise follows user preference */
  showBrandAvatar?: boolean
  className?: string
}

const SIZE_CLASSES = {
  xs: 'h-6 w-6 text-[11px]',
  sm: 'h-8 w-8 text-[12px]',
  md: 'h-9 w-9 text-[13px]',
  lg: 'h-10 w-10 text-[14px]',
} as const

export function SenderAvatar({
  name,
  email,
  size = 'md',
  verified = false,
  showBrandAvatar,
  className,
}: SenderAvatarProps) {
  const prefs = useContext(PreferencesContext)
  const brandEnabled = showBrandAvatar ?? prefs?.showBrandAvatars ?? true
  const brand = verified && brandEnabled ? matchBrandByEmail(email) : null

  if (brand) {
    if (brand.mode === 'monogram' || !brand.svg) {
      return (
        <div
          title={brand.name}
          aria-label={brand.name}
          data-brand-avatar={brand.id}
          style={{ backgroundColor: brand.color }}
          className={cn(
            'inline-flex shrink-0 items-center justify-center rounded-full font-semibold text-white select-none',
            SIZE_CLASSES[size],
            className,
          )}
        >
          {brand.monogram}
        </div>
      )
    }

    const isMono = brand.mode === 'mono'
    return (
      <div
        title={brand.name}
        aria-label={brand.name}
        data-brand-avatar={brand.id}
        style={isMono ? { color: brand.color } : undefined}
        className={cn(
          'inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full border border-black/[0.06] bg-[#f3f4f6] select-none dark:border-white/[0.08] dark:bg-[#232732]',
          brand.darkLight && isMono && 'dark:!text-slate-100',
          brand.darkLight && !isMono && '[&>svg]:dark:brightness-0 [&>svg]:dark:invert',
          '[&>svg]:h-[60%] [&>svg]:w-[60%] [&>svg]:shrink-0',
          SIZE_CLASSES[size],
          className,
        )}
        // Build-time extracted and strictly sanitized static SVG from src/generated/brandIcons.ts
        dangerouslySetInnerHTML={{ __html: brand.svg }}
      />
    )
  }

  const palette = getAvatarPalette(email || name || 'pmail')
  const initials = getInitials(name, email)
  return (
    <div
      aria-hidden="true"
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-full font-semibold select-none',
        SIZE_CLASSES[size],
        palette.bg,
        palette.text,
        className,
      )}
    >
      {initials}
    </div>
  )
}
