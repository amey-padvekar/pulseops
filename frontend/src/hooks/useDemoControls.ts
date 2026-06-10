import { useCallback, useState } from 'react'

import { useApiBaseUrl } from './useApiBaseUrl'
import type { DemoScenario } from '../data/demoScenarios'

type DemoStatus = 'idle' | 'submitting' | 'success' | 'error'

export type DemoIncidentResponse = {
  incidentId: string
  deviceId: string
  serviceName: string
  scenario: string
  state: string
}

type UseDemoControlsResult = {
  status: DemoStatus
  error: string | null
  response: DemoIncidentResponse | null
  submitting: boolean
  /** Simulate a service failure on the selected device (POST /demo/incident). */
  generate: (deviceId: string, scenario: DemoScenario, autoApprove: boolean) => Promise<boolean>
  /** Clear the device's demo incidents so a scenario can be re-run (POST /demo/reset). */
  reset: (deviceId: string) => Promise<boolean>
  /** Reset local status/error/response (does not call the backend). */
  clear: () => void
}

/**
 * useDemoControls drives the DEMO_MODE endpoints and tracks submission state so the panel can
 * disable controls and surface success/error feedback. It mirrors useApproveIncident. The two
 * actions share one status because the panel performs one at a time.
 */
export function useDemoControls(): UseDemoControlsResult {
  const apiBaseUrl = useApiBaseUrl()
  const [status, setStatus] = useState<DemoStatus>('idle')
  const [error, setError] = useState<string | null>(null)
  const [response, setResponse] = useState<DemoIncidentResponse | null>(null)

  const clear = useCallback(() => {
    setStatus('idle')
    setError(null)
    setResponse(null)
  }, [])

  const generate = useCallback(
    async (deviceId: string, scenario: DemoScenario, autoApprove: boolean): Promise<boolean> => {
      setStatus('submitting')
      setError(null)
      setResponse(null)

      try {
        const res = await fetch(`${apiBaseUrl}/demo/incident`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          // Send the scenario key (drives the flavored logs) AND the serviceName (lets the
          // dropdown target the exact installed service); the backend uses both.
          body: JSON.stringify({
            deviceId,
            scenario: scenario.key,
            serviceName: scenario.serviceName,
            autoApprove,
          }),
        })

        if (!res.ok) {
          setStatus('error')
          setError(await readError(res, `Simulate failed (${res.status})`))
          return false
        }

        const data = (await res.json()) as DemoIncidentResponse
        setResponse(data)
        setStatus('success')
        return true
      } catch (err) {
        setStatus('error')
        setError(err instanceof Error ? err.message : 'Network error during simulate')
        return false
      }
    },
    [apiBaseUrl],
  )

  const reset = useCallback(
    async (deviceId: string): Promise<boolean> => {
      setStatus('submitting')
      setError(null)
      setResponse(null)

      try {
        const res = await fetch(`${apiBaseUrl}/demo/reset`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ deviceId }),
        })

        if (!res.ok) {
          setStatus('error')
          setError(await readError(res, `Reset failed (${res.status})`))
          return false
        }

        setStatus('success')
        return true
      } catch (err) {
        setStatus('error')
        setError(err instanceof Error ? err.message : 'Network error during reset')
        return false
      }
    },
    [apiBaseUrl],
  )

  return {
    status,
    error,
    response,
    submitting: status === 'submitting',
    generate,
    reset,
    clear,
  }
}

// readError extracts a human message from a failed response. The demo endpoints use Go's
// http.Error (plain-text body); approval-style JSON {error} bodies are also handled.
async function readError(res: Response, fallback: string): Promise<string> {
  try {
    const text = (await res.text()).trim()
    if (text) {
      try {
        const json = JSON.parse(text) as { error?: string }
        if (json?.error) {
          return json.error
        }
      } catch {
        // Not JSON — fall through to the plain-text body.
      }
      return text
    }
  } catch {
    // Body unreadable; use the fallback.
  }
  return fallback
}
