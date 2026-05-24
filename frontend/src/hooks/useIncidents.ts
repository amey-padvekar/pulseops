import { useEffect, useRef, useState } from 'react'

import { useApiBaseUrl, useWsBaseUrl } from './useApiBaseUrl'
import type { Incident, WsEventEnvelope } from '../types/dashboard'

type UseIncidentsResult = {
  incidents: Incident[]
  activeIncident: Incident | null
  connected: boolean
}

function isIncident(value: unknown): value is Incident {
  if (!value || typeof value !== 'object') {
    return false
  }
  const incident = value as Partial<Incident>
  return (
    typeof incident.incidentId === 'string' &&
    typeof incident.deviceId === 'string' &&
    typeof incident.state === 'string' &&
    typeof incident.severity === 'string' &&
    typeof incident.active === 'boolean'
  )
}

function upsertIncident(previous: Incident[], next: Incident): Incident[] {
  const existingIndex = previous.findIndex((item) => item.incidentId === next.incidentId)
  if (existingIndex === -1) {
    return [next, ...previous].sort(
      (a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
    )
  }

  const updated = previous.slice()
  updated[existingIndex] = next
  updated.sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime())
  return updated
}

export function useIncidents(deviceId?: string): UseIncidentsResult {
  const apiBaseUrl = useApiBaseUrl()
  const wsBaseUrl = useWsBaseUrl()

  const [incidents, setIncidents] = useState<Incident[]>([])
  const [connected, setConnected] = useState(false)

  const socketRef = useRef<WebSocket | null>(null)
  const reconnectTimerRef = useRef<number | null>(null)

  useEffect(() => {
    let isMounted = true

    const clearReconnectTimer = () => {
      if (reconnectTimerRef.current !== null) {
        window.clearTimeout(reconnectTimerRef.current)
        reconnectTimerRef.current = null
      }
    }

    const fetchIncidents = async () => {
      try {
        const response = await fetch(`${apiBaseUrl}/incidents?active=true`)
        if (!response.ok) {
          return
        }
        const data = (await response.json()) as Incident[]
        if (!isMounted || !Array.isArray(data)) {
          return
        }

        const normalized = data
          .filter((incident) => !deviceId || incident.deviceId === deviceId)
          .sort(
            (a, b) =>
              new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
          )
        setIncidents(normalized)
      } catch {
        // Best effort fallback.
      }
    }

    const scheduleReconnect = () => {
      if (!isMounted || reconnectTimerRef.current !== null) {
        return
      }

      reconnectTimerRef.current = window.setTimeout(() => {
        reconnectTimerRef.current = null
        connect()
      }, 3000)
    }

    const connect = () => {
      if (!isMounted) {
        return
      }

      const ws = new WebSocket(`${wsBaseUrl}/ws`)
      socketRef.current = ws

      ws.onopen = () => {
        if (isMounted) {
          setConnected(true)
        }
      }

      ws.onmessage = (event) => {
        let parsed: unknown
        try {
          parsed = JSON.parse(event.data as string)
        } catch {
          return
        }

        if (!parsed || typeof parsed !== 'object') {
          return
        }

        const envelope = parsed as Partial<WsEventEnvelope<unknown>>
        if (envelope.type !== 'incident.updated' || !isIncident(envelope.payload)) {
          return
        }

        const incoming = envelope.payload
        if (deviceId && incoming.deviceId !== deviceId) {
          return
        }

        setIncidents((previous) => upsertIncident(previous, incoming))
      }

      ws.onclose = () => {
        if (!isMounted) {
          return
        }

        setConnected(false)
        scheduleReconnect()
      }

      ws.onerror = () => {
        if (!isMounted) {
          return
        }
        setConnected(false)
      }
    }

    void fetchIncidents()
    connect()

    return () => {
      isMounted = false
      setConnected(false)
      clearReconnectTimer()

      const socket = socketRef.current
      socketRef.current = null
      if (
        socket &&
        (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)
      ) {
        socket.close()
      }
    }
  }, [apiBaseUrl, deviceId, wsBaseUrl])

  const activeIncident = incidents.find((incident) => incident.active) ?? null

  return {
    incidents,
    activeIncident,
    connected,
  }
}
