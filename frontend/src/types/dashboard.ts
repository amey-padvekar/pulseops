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

export type IncidentState =
  | 'healthy'
  | 'detected'
  | 'investigating'
  | 'awaiting_approval'
  | 'approved'
  | 'executing'
  | 'validating'
  | 'resolved'
  | 'failed'

export type Severity = 'low' | 'medium' | 'high' | 'critical'

export type Incident = {
  incidentId: string
  deviceId: string
  serviceName: string
  serviceStatus: ServiceStatus
  state: IncidentState
  severity: Severity
  createdAt: string
  updatedAt: string
  detectedAt: string
  lastSeenAt: string
  reason: string
  active: boolean
}

export type WsEventEnvelope<TPayload> = {
  type: string
  payload: TPayload
}
