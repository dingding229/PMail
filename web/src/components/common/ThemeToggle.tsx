import { Check, Monitor, Moon, Sun } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useTheme, type ThemeMode } from '@/lib/theme'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

interface ThemeToggleProps {
  align?: 'start' | 'center' | 'end'
}

export function ThemeToggle({ align = 'end' }: ThemeToggleProps) {
  const { t } = useTranslation()
  const { mode, resolved, setMode } = useTheme()

  const options: { value: ThemeMode; label: string; icon: typeof Sun }[] = [
    { value: 'light', label: t('theme.light'), icon: Sun },
    { value: 'dark', label: t('theme.dark'), icon: Moon },
    { value: 'system', label: t('theme.system'), icon: Monitor },
  ]

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('theme.label')}
          title={t('theme.label')}
        >
          {resolved === 'dark' ? (
            <Moon className="h-4 w-4" />
          ) : (
            <Sun className="h-4 w-4" />
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={align}>
        {options.map(({ value, label, icon: Icon }) => (
          <DropdownMenuItem
            key={value}
            onClick={() => setMode(value)}
            className="justify-between"
          >
            <span className="inline-flex items-center gap-2">
              <Icon className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              {label}
            </span>
            {mode === value && <Check className="h-3.5 w-3.5 text-[var(--accent)]" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
