import { useEffect, useRef, useState } from 'react'

import { useApiBaseUrl, useWsBaseUrl } from './useApiBaseUrl'
import type { DeviceState, WsEventEnvelope } from '../types/dashboard'

type UseDeviceStateResult = {
  deviceState: DeviceState | null
  connected: boolean
}

export function useDeviceState(deviceId: string): UseDeviceStateResult {
  const apiBaseUrl = useApiBaseUrl()
  const wsBaseUrl = useWsBaseUrl()

  const [deviceState, setDeviceState] = useState<DeviceState | null>(null)
  const [connected, setConnected] = useState(false)

  const socketRef = useRef<WebSocket | null>(null)
  const reconnectTimerRef = useRef<number | null>(null)
  const wasEverConnectedRef = useRef(false)
  const fallbackFetchedRef = useRef(false)

  useEffect(() => {
    let isMounted = true

    const clearReconnectTimer = () => {
      if (reconnectTimerRef.current !== null) {
        window.clearTimeout(reconnectTimerRef.current)
        reconnectTimerRef.current = null
      }
    }

    const fetchFallbackOnce = async () => {
      if (fallbackFetchedRef.current) {
        return
      }
      fallbackFetchedRef.current = true

      try {
        const response = await fetch(
          `${apiBaseUrl}/devices/${encodeURIComponent(deviceId)}`,
        )
        if (!response.ok) {
          return
        }

        const data = (await response.json()) as DeviceState
        if (!isMounted) {
          return
        }
        if (typeof data?.deviceId === 'string' && data.deviceId === deviceId) {
          setDeviceState(data)
        }
      } catch {
        // Fallback is best-effort.
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
        wasEverConnectedRef.current = true
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

        if (!isMounted || !parsed || typeof parsed !== 'object') {
          return
        }

        let maybeState: Partial<DeviceState> | null = null

        const envelope = parsed as Partial<WsEventEnvelope<unknown>>
        if (
          envelope.type === 'telemetry.updated' &&
          envelope.payload &&
          typeof envelope.payload === 'object'
        ) {
          maybeState = envelope.payload as Partial<DeviceState>
        } else {
          maybeState = parsed as Partial<DeviceState>
        }

        if (maybeState.deviceId === deviceId) {
          setDeviceState(maybeState as DeviceState)
        }
      }

      ws.onerror = () => {
        if (!wasEverConnectedRef.current) {
          void fetchFallbackOnce()
        }
      }

      ws.onclose = () => {
        if (!isMounted) {
          return
        }

        setConnected(false)
        if (!wasEverConnectedRef.current) {
          void fetchFallbackOnce()
        }
        scheduleReconnect()
      }
    }

    connect()

    return () => {
      isMounted = false
      setConnected(false)
      clearReconnectTimer()

      const socket = socketRef.current
      socketRef.current = null

      if (
        socket &&
        (socket.readyState === WebSocket.OPEN ||
          socket.readyState === WebSocket.CONNECTING)
      ) {
        socket.close()
      }
    }
  }, [apiBaseUrl, deviceId, wsBaseUrl])

  return { deviceState, connected }
}
