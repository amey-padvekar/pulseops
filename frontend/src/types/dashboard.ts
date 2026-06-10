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
  // AI investigation fields (Phase 7)
  probableCause?: string
  confidence?: number
  recommendedActions?: Array<{
    actionId: string
    target?: string
    description?: string
  }>
  validationSteps?: string[]
  summary?: string
  investigatedAt?: string
  investigationStatus?: string
  investigationError?: string
  agentBuilderTraceId?: string
  // Approval metadata (Phase 8)
  approvedBy?: string
  approvedAt?: string
  approvalNote?: string
  approvedActions?: string[]
  // Remediation execution outcome (Phase 9)
  remediationStatus?: string
  remediationRequestId?: string
  remediationStartedAt?: string
  remediationFinishedAt?: string
  remediationReceivedAt?: string
  remediationResults?: RemediationActionResult[]
  // Execution timeline (Phase 9)
  timeline?: TimelineEvent[]
  // Recovery validation (Phase 10)
  validationBoundaryAt?: string
  healthyCycleCount?: number
  requiredHealthyCycles?: number
  lastValidationTelemetryAt?: string
  lastValidationReason?: string
  validationFailureReason?: string
  validationStatus?: ValidationStatus
  validatedAt?: string
  lastValidationSnapshot?: ValidationSnapshot
  // Final summary (Phase 11)
  finalSummary?: FinalSummary
  summaryStatus?: SummaryStatus
  summaryGeneratedAt?: string
  summaryRequestId?: string
}

export type ValidationStatus = 'in_progress' | 'succeeded' | 'failed'

export type SummaryStatus = 'pending' | 'generated' | 'fallback' | 'failed'

export type FinalSummary = {
  rootCause: string
  evidence: string[]
  actionsTaken: string[]
  result: string
  operatorSummary?: string
}

export type HealthCheck = {
  name: string
  passed: boolean
  required: boolean
  detail: string
}

export type ValidationSnapshot = {
  observedAt: string
  healthy: boolean
  reason: string
  checks: HealthCheck[]
  serviceStatus: ServiceStatus | string
  heartbeat: boolean
  networkReachable: boolean
}

export type RemediationActionResult = {
  actionId: string
  target?: string
  status: string
  stdout?: string
  stderr?: string
  exitCode?: number
  durationMs: number
}

export type TimelineEventType =
  | 'command_queued'
  | 'command_dispatched'
  | 'command_started'
  | 'command_finished'

export type TimelineEvent = {
  type: TimelineEventType
  at: string
  detail?: string
}

export type ApprovalRequest = {
  approvedBy: string
  selectedActionIds: string[]
  note?: string
}

export type QueuedAction = {
  actionId: string
  target?: string
}

export type ApprovalResponse = {
  incidentId: string
  state: IncidentState
  approvedBy: string
  approvedAt: string
  queuedActions: QueuedAction[]
}

export type WsEventEnvelope<TPayload> = {
  type: string
  payload: TPayload
}
