export function useApiBaseUrl(): string {
  return import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'
}

export function useWsBaseUrl(): string {
  const apiBase = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'
  return apiBase.replace(/^http/, 'ws')
}
