import { useEffect, useState } from 'react'

import { useApiBaseUrl } from './useApiBaseUrl'

type StatsResponse = {
  activeAgents: number
  activeAgentDevices?: string[]
}

type UseAgentStatsResult = {
  activeAgents: number
  activeAgentDevices: string[]
}

export function useAgentStats(): UseAgentStatsResult {
  const apiBaseUrl = useApiBaseUrl()
  const [activeAgents, setActiveAgents] = useState(0)
  const [activeAgentDevices, setActiveAgentDevices] = useState<string[]>([])

  useEffect(() => {
    let isMounted = true

    const fetchStats = async () => {
      try {
        const response = await fetch(`${apiBaseUrl}/stats`)
        if (!response.ok) {
          return
        }

        const data = (await response.json()) as StatsResponse
        if (!isMounted) {
          return
        }

        let resolvedDevices: string[] = []
        if (Array.isArray(data.activeAgentDevices)) {
          resolvedDevices = data.activeAgentDevices.filter(
            (value): value is string => typeof value === 'string',
          )
        }

        if (resolvedDevices.length === 0) {
          const devicesResponse = await fetch(`${apiBaseUrl}/devices`)
          if (devicesResponse.ok) {
            const devicesData = (await devicesResponse.json()) as Array<{ deviceId?: string }>
            resolvedDevices = devicesData
              .map((device) => (typeof device.deviceId === 'string' ? device.deviceId : ''))
              .filter((value) => value.length > 0)
          }
        }

        if (typeof data.activeAgents === 'number') {
          setActiveAgents(resolvedDevices.length > 0 ? resolvedDevices.length : data.activeAgents)
        } else {
          setActiveAgents(resolvedDevices.length)
        }
        setActiveAgentDevices(resolvedDevices)
      } catch {
        // Best-effort stats panel update.
      }
    }

    void fetchStats()
    const intervalId = window.setInterval(() => {
      void fetchStats()
    }, 5000)

    return () => {
      isMounted = false
      window.clearInterval(intervalId)
    }
  }, [apiBaseUrl])

  return { activeAgents, activeAgentDevices }
}