import { useCallback, useState } from 'react'

import { useApiBaseUrl } from './useApiBaseUrl'
import type { ApprovalRequest, ApprovalResponse } from '../types/dashboard'

type ApproveStatus = 'idle' | 'submitting' | 'success' | 'error'

type UseApproveIncidentResult = {
  status: ApproveStatus
  error: string | null
  response: ApprovalResponse | null
  submitting: boolean
  approve: (incidentId: string, request: ApprovalRequest) => Promise<boolean>
  reset: () => void
}

/**
 * useApproveIncident posts an approval to POST /incidents/{id}/approve and tracks
 * submission state so the UI can disable controls and surface success/error feedback.
 */
export function useApproveIncident(): UseApproveIncidentResult {
  const apiBaseUrl = useApiBaseUrl()
  const [status, setStatus] = useState<ApproveStatus>('idle')
  const [error, setError] = useState<string | null>(null)
  const [response, setResponse] = useState<ApprovalResponse | null>(null)

  const reset = useCallback(() => {
    setStatus('idle')
    setError(null)
    setResponse(null)
  }, [])

  const approve = useCallback(
    async (incidentId: string, request: ApprovalRequest): Promise<boolean> => {
      setStatus('submitting')
      setError(null)
      setResponse(null)

      try {
        const res = await fetch(
          `${apiBaseUrl}/incidents/${encodeURIComponent(incidentId)}/approve`,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(request),
          },
        )

        if (!res.ok) {
          let message = `Approval failed (${res.status})`
          try {
            const body = (await res.json()) as { error?: string }
            if (body?.error) {
              message = body.error
            }
          } catch {
            // Non-JSON error body; keep the status-based message.
          }
          setStatus('error')
          setError(message)
          return false
        }

        const data = (await res.json()) as ApprovalResponse
        setResponse(data)
        setStatus('success')
        return true
      } catch (err) {
        setStatus('error')
        setError(err instanceof Error ? err.message : 'Network error during approval')
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
    approve,
    reset,
  }
}
