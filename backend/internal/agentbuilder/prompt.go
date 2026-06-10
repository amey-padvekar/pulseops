package agentbuilder

// PromptTemplate is the Gemini/Agent Builder prompt template used to request an
// investigation. The backend should populate the placeholders with the compact
// evidence summary retrieved via Elastic MCP.
//
// Template placeholders:
//  - {{.IncidentID}}
//  - {{.DeviceID}}
//  - {{.Service}} (optional)
//  - {{.TimeWindow}}
//  - {{.EvidenceSummary}} (compact list or bullet points)
//
// IMPORTANT: workflows must return strictly-parseable JSON that matches
// the 'InvestigationResult' shape defined in investigation_model.go.
const PromptTemplate = `You are an investigation assistant. Use ONLY the evidence provided below to identify a probable cause, recommend safe remediation action IDs, and list validation steps.

Input metadata:
- incidentId: {{.IncidentID}}
- deviceId: {{.DeviceID}}
- service: {{.Service}}
- timeWindow: {{.TimeWindow}}

Evidence (use these facts only):
{{.EvidenceSummary}}

Rules (follow exactly):
1) Reason only from the evidence shown above. Do not invent logs, telemetry, or external facts.
2) Output must be valid JSON strictly matching the 'InvestigationResult' schema.
3) 'recommendedActions' must contain only approved 'actionId' values. Do NOT include shell commands.
4) Keep 'summary' and 'probableCause' concise and operator-facing (one short paragraph maximum).
5) 'confidence' must be a number between 0.0 and 1.0 indicating your estimated confidence.

Decision heuristics (MVP guidance):
- If the evidence shows the monitored service is stopped but heartbeat telemetry from the same host is present, prefer 'probableCause' = "service stopped" and include 'restart_service' in 'recommendedActions' when justified by logs.

Output example (JSON only):
{
  "probableCause": "string",
  "confidence": 0.0,
  "recommendedActions": [
    {"actionId": "restart_service", "target": "service-name or device-id", "reason": "concise reason"}
  ],
  "validationSteps": ["step 1 to validate", "step 2"],
  "summary": "short operator-facing summary"
}
`
