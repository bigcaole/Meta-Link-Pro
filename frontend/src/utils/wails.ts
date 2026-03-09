import type { GenerateMetaYAMLRequest, ParseReport, ServiceTree } from '../types'

type WailsLike = Record<string, (...args: unknown[]) => Promise<unknown>>

function resolveAppBinding(): WailsLike | null {
  const globalAny = window as unknown as Record<string, unknown>
  const candidates = [
    (globalAny.backend as Record<string, unknown> | undefined)?.App,
    ((globalAny.go as Record<string, unknown> | undefined)?.backend as Record<string, unknown> | undefined)?.App,
    ((globalAny.go as Record<string, unknown> | undefined)?.main as Record<string, unknown> | undefined)?.App
  ]

  for (const item of candidates) {
    if (item && typeof item === 'object') {
      return item as WailsLike
    }
  }
  return null
}

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const app = resolveAppBinding()
  if (!app || typeof app[method] !== 'function') {
    throw new Error('未找到 Wails 后端绑定，请确认已在 Wails v3 里注入 App 服务。')
  }
  return (await app[method](...args)) as T
}

export async function parseLinks(input: string): Promise<ParseReport> {
  return call<ParseReport>('ParseLinks', input)
}

export async function loadServiceTree(): Promise<ServiceTree[]> {
  return call<ServiceTree[]>('LoadServiceTree')
}

export async function generateMetaYAML(payload: GenerateMetaYAMLRequest): Promise<string> {
  return call<string>('GenerateMetaYAML', payload)
}

export async function exportToDesktop(content: string): Promise<string> {
  return call<string>('ExportToDesktop', content)
}
