import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App'
import {
  configureColorScheme,
  LocalStorageColorSchemeResolver,
} from '@/hooks/color-scheme/color-scheme'

// Configure the react-kit color-scheme store before any hook invocation.
// strategy: "data-attribute" matches the existing CSS which keys on [data-theme].
// The FOUC-blocker script injected into <head> by vite.config.ts uses the same
// storageKey so there is one source of truth.
configureColorScheme({
  strategy: 'data-attribute',
  resolver: new LocalStorageColorSchemeResolver({ storageKey: 'radix-metrics-theme' }),
})

const rootEl = document.getElementById('root')
if (!rootEl) throw new Error('Root element not found')

createRoot(rootEl).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
