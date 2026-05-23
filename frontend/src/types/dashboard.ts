export type ServiceStatus = 'running' | 'stopped' | 'degraded' | 'unknown'

export type DeviceState = {
  deviceId: string
  timestamp: string
  serviceName: string
  serviceStatus: ServiceStatus
  networkReachable: boolean
  cpuUsage: number
  memoryUsage: number
  recentLogs: string[]
  heartbeat: boolean
  lastSeenAt: string
}

export type CardStatus = 'placeholder' | 'healthy' | 'degraded' | 'stopped' | 'unknown'

export type DashboardCard = {
  title: string
  status: CardStatus
  description: string
}
