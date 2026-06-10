/**
 * useDemoMode reports whether the judge-facing DEMO_MODE panel ("Simulate Service Failure")
 * should render. Mirrors the env-driven useApiBaseUrl; set VITE_DEMO_MODE=true in the
 * hosted/demo build to enable. Off by default so production never shows demo controls.
 */
export function useDemoMode(): boolean {
  return import.meta.env.VITE_DEMO_MODE === 'true'
}
