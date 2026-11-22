import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './i18n'
import './styles/index.css'
import { App } from './App'

async function enableMocking() {
  if (import.meta.env.VITE_MOCK === '1') {
    const { worker } = await import('./mocks/browser')
    await worker.start({
      onUnhandledRequest: 'bypass',
      serviceWorker: {
        url: '/mockServiceWorker.js',
      },
    })
  }
}

void enableMocking().then(() => {
  const rootEl = document.getElementById('root')
  if (!rootEl) return
  createRoot(rootEl).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
})
