import { Component, type ErrorInfo, type ReactNode } from 'react'
import { AlertTriangle, RotateCcw } from 'lucide-react'
import i18n from '@/i18n'
import { Button } from '@/components/ui/button'

interface ErrorBoundaryProps {
  children: ReactNode
  /** Changing this key resets the caught error state (e.g. activeEmailId or route pathname) */
  resetKey?: string | number | null
  /** Compact inline layout for sub-panes such as ReadingPane or Settings sections */
  inline?: boolean
}

interface ErrorBoundaryState {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = {
    hasError: false,
    error: null,
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    if (import.meta.env.DEV) {
      console.error('[PMail ErrorBoundary]', error, info.componentStack)
    }
  }

  componentDidUpdate(prevProps: ErrorBoundaryProps): void {
    if (this.state.hasError && prevProps.resetKey !== this.props.resetKey) {
      this.setState({ hasError: false, error: null })
    }
  }

  private handleReset = () => {
    this.setState({ hasError: false, error: null })
  }

  render() {
    if (!this.state.hasError) {
      return this.props.children
    }

    const t = i18n.t.bind(i18n)
    const { inline = false } = this.props

    return (
      <div
        role="alert"
        className={
          inline
            ? 'flex h-full min-h-[240px] flex-col items-center justify-center rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] px-6 py-10 text-center'
            : 'flex min-h-full flex-col items-center justify-center bg-[var(--surface-canvas)] px-6 py-16 text-center'
        }
      >
        <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-[var(--danger-soft)] text-[var(--danger)]">
          <AlertTriangle className="h-5 w-5" />
        </div>
        <h2 className="mt-4 text-[15.5px] font-semibold text-[var(--ink-primary)]">
          {t('app.errorBoundaryTitle')}
        </h2>
        <p className="mt-1.5 max-w-md text-[13px] leading-relaxed text-[var(--ink-secondary)]">
          {t('app.errorBoundaryDesc')}
        </p>
        {this.state.error?.message && (
          <p className="mt-2 max-w-md break-all rounded-[var(--radius-sm)] bg-[var(--surface-subtle)] px-3 py-1.5 font-mono text-[11.5px] text-[var(--ink-muted)]">
            {this.state.error.message}
          </p>
        )}
        <div className="mt-5 flex flex-wrap items-center justify-center gap-2">
          <Button variant="primary" size="sm" onClick={this.handleReset}>
            <RotateCcw className="h-3.5 w-3.5" />
            {t('app.retry')}
          </Button>
          {!inline && (
            <Button variant="outline" size="sm" onClick={() => window.location.reload()}>
              {t('app.reloadPage')}
            </Button>
          )}
        </div>
      </div>
    )
  }
}
