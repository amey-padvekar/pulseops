# AI Auto-Healing Incident Response Agent

## Prerequisites (Phase 1)

Install these tools before first run.

| Tool | Required Version | Check Command |
|---|---|---|
| Git | 2.40+ | `git --version` |
| Go | 1.22+ | `go version` |
| Node.js | 22 LTS+ | `node --version` |
| npm | 10+ | `npm --version` |
| PowerShell | 5.1+ (Windows) or 7+ | `$PSVersionTable.PSVersion` |

## First-Time Clone and Run

Run from PowerShell.

1. Clone repository

```powershell
git clone <your-repo-url>
cd pulseops
```

2. Create local env files from examples

```powershell
Copy-Item .\agent\.env.example .\agent\.env
Copy-Item .\backend\.env.example .\backend\.env
Copy-Item .\frontend\.env.example .\frontend\.env
```

3. Install dependencies

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-deps.ps1
```

4. Verify baseline startup

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\smoke-check.ps1
```

5. Start components

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-backend.ps1
```

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-agent.ps1
```

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-frontend.ps1
```

## Phase 1 Quickstart (Local)

Use PowerShell from the repository root.

1. Install dependencies

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-deps.ps1
```

2. Start components (separate terminals)

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-backend.ps1
```

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-agent.ps1
```

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-frontend.ps1
```

Optional: launch all three in new terminals

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-all.ps1
```

3. Run smoke check

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\smoke-check.ps1
```

Expected smoke check result:
- agent builds
- backend builds
- frontend builds
- backend `/healthz` returns `{"status":"ok"}`

## Platform Assumptions (Demo)

- Primary demo path is Windows.
- Scripts in `scripts/` are PowerShell-first for reproducible Windows setup.
- Linux support can be added later, but Phase 1 acceptance is validated on Windows.

## Troubleshooting (Phase 1)

1. `go` command not found
- Ensure Go is installed and available in `PATH`.

2. `npm` or `node` command not found
- Install Node.js LTS and reopen terminal.

3. Frontend port conflict (`5173` already in use)
- Stop existing process on 5173 or run frontend with a different port.

4. Backend health check fails during smoke-check
- Confirm no process is already bound to `8080`.
- Run `powershell -ExecutionPolicy Bypass -File .\scripts\run-backend.ps1` and verify `http://localhost:8080/healthz` returns status `ok`.

5. PowerShell execution policy blocks script execution
- Run scripts using `-ExecutionPolicy Bypass` as shown above.

6. `.env` values not applied
- Verify `.env` files exist under `agent/`, `backend/`, and `frontend/`.
- Confirm variable names match the examples exactly.

## Overview

An autonomous AI-powered IT operations and incident remediation system built for the Google Cloud Rapid Agent Hackathon.

The system continuously monitors endpoint health, analyzes telemetry and logs, detects operational issues, identifies probable root causes using Gemini, recommends remediation actions, and optionally executes approved fixes automatically.

The project focuses on:

* AI-driven incident response
* Endpoint monitoring
* Auto-remediation
* Real-time telemetry
* Human-in-the-loop automation
* Enterprise operations workflows

---

# Problem Statement

Modern IT teams spend significant time manually investigating and resolving repetitive operational incidents such as:

* VPN failures
* Service crashes
* Failed deployments
* High CPU or memory usage
* Endpoint instability
* Connectivity issues
* Log analysis and root cause identification

This leads to:

* Increased downtime
* Alert fatigue
* Slow incident response
* Operational inefficiency
* Large support queues

The goal of this project is to create an AI agent capable of:

1. Detecting operational issues
2. Investigating root causes
3. Suggesting remediation
4. Executing approved fixes
5. Verifying recovery
6. Generating incident summaries

---

# Core Idea

Instead of building a chatbot, the project demonstrates:

> Autonomous AI systems that take operational actions.

The AI agent behaves like an intelligent operations engineer.

---

# Hackathon Alignment

This project aligns strongly with the Google Cloud Rapid Agent Hackathon judging criteria.

## Technological Implementation

* WebSocket orchestration
* Endpoint agents
* Telemetry pipeline
* Gemini reasoning
* Auto-remediation workflows
* Real-time dashboards

## Design

* Clear incident workflow
* Live remediation visualization
* Human approval flows
* Operational dashboard

## Potential Impact

* Real enterprise use case
* Reduces operational workload
* Improves response times
* Reduces downtime

## Quality of Idea

* Autonomous incident remediation
* AI-driven operational reasoning
* Multi-step workflows
* Real-time orchestration

---

# Recommended Hackathon Scope

Keep the project intentionally focused.

## Recommended MVP

Build:

* One endpoint agent
* One backend service
* One dashboard
* One remediation workflow

Do NOT attempt:

* Large-scale distributed systems
* Offline endpoint recovery
* Enterprise-grade security hardening
* Complex peer-to-peer networking
* Full ITSM integrations

---

# Recommended Demo Scenario

## AI Auto-Healing VPN Incident

This is the recommended primary demo because it is:

* Easy to understand
* Easy to demonstrate
* Visually clear
* Real-world relevant
* Technically manageable

---

# Demo Flow

## Step 1 — Healthy State

Dashboard shows:

* VPN connected
* Device healthy
* Services running

Everything appears green.

---

## Step 2 — Simulated Failure

Manually stop the VPN service:

```powershell
Stop-Service OpenVPNService
```

Alternative failures:

* Stop backend service
* Simulate DNS failure
* Kill a monitored process
* Generate fake error logs

Important:
The endpoint itself remains online.
Only the monitored service becomes unhealthy.

---

## Step 3 — Monitoring Agent Detects Issue

The endpoint agent sends telemetry:

```json
{
  "vpnConnected": false,
  "serviceStatus": "stopped",
  "networkReachable": true
}
```

---

## Step 4 — AI Investigation

Gemini receives:

* Service state
* Telemetry
* Recent logs
* Historical context

AI determines probable cause.

Example:

> VPN connectivity failure likely caused by stopped OpenVPN service.

---

## Step 5 — AI Suggests Remediation

Dashboard displays:

* Restart VPN service
* Flush DNS cache
* Reconnect VPN

---

## Step 6 — Human Approval

Admin clicks:

"Approve Remediation"

This is important because enterprise systems usually require human oversight.

---

## Step 7 — AI Executes Fix

Example commands:

```powershell
Restart-Service OpenVPNService
ipconfig /flushdns
```

---

## Step 8 — Recovery Validation

The endpoint agent reports:

```json
{
  "vpnConnected": true,
  "serviceStatus": "running"
}
```

Dashboard returns to healthy state.

---

## Step 9 — Incident Summary

AI generates a final report.

Example:

```text
Incident Summary

Root Cause:
VPN service unexpectedly stopped.

Actions Taken:
- Restarted VPN service
- Flushed DNS cache
- Revalidated connectivity

Result:
VPN connectivity restored successfully.
```

---

# Alternative Demo Scenarios

## Deployment Rollback Agent

Scenario:

* New deployment causes failures
* Error rates spike
* AI identifies issue
* AI rolls back deployment
* Service recovers

---

## Auto-Healing Service Monitor

Scenario:

* Backend service crashes
* AI detects unhealthy state
* AI restarts service
* AI validates recovery

---

## AI Log Investigator

Scenario:

* Application fails
* AI analyzes logs
* AI identifies probable root cause
* AI suggests remediation

---

# System Architecture

## High-Level Architecture

```text
Endpoint Agent
      ↓
Telemetry + Logs
      ↓
Backend / Telemetry Layer
      ↓
Elastic
      ↓
Elastic MCP Server
      ↓
Google Cloud Agent Builder
      ↓
Gemini Reasoning + Workflow Orchestration
      ↓
Remediation Workflow
      ↓
Endpoint Executes Fix
```

---

# Google Cloud Agent Builder Workflow

## Why Agent Builder Is Critical

The Google Cloud Rapid Agent Hackathon explicitly requires meaningful usage of:

* Gemini
* Google Cloud Agent Builder
* Partner MCP servers

This project therefore uses Google Cloud Agent Builder as the primary orchestration and reasoning layer instead of calling Gemini directly from the backend.

Agent Builder is responsible for:

* Workflow orchestration
* Incident investigation
* Tool usage
* MCP interactions
* Remediation planning
* Incident summarization

---

# Agent Builder Responsibilities

The Agent Builder workflow acts as the autonomous operations coordinator.

It receives:

* telemetry
* incidents
* logs
* endpoint health information

and performs:

* investigation
* reasoning
* remediation planning
* tool interactions
* validation workflows

---

# Agent Builder Operational Flow

## Step 1 — Incident Trigger

The endpoint agent detects a service failure and forwards telemetry to the backend.

Example:

```json
{
  "device": "LAPTOP-22",
  "serviceStatus": "stopped",
  "vpnConnected": false,
  "severity": "high"
}
```

The backend forwards the operational context into the Agent Builder workflow.

---

## Step 2 — Agent Builder Invokes Elastic MCP Tools

The Agent Builder agent uses Elastic MCP tools to:

* search logs
* retrieve recent incidents
* correlate telemetry
* identify repeated failures
* analyze historical patterns

Example MCP-style queries:

```text
Search logs for:
"VPN service terminated unexpectedly"
```

```text
Retrieve all incidents for device LAPTOP-22 in the last 15 minutes
```

---

## Step 3 — Gemini Operational Reasoning

Gemini analyzes:

* logs
* telemetry
* incident history
* operational patterns

Gemini determines probable root cause.

Example:

> VPN connectivity failure likely caused by stopped OpenVPN service after recent update.

---

## Step 4 — Remediation Planning

Agent Builder generates a remediation workflow.

Example remediation plan:

1. Restart VPN service
2. Flush DNS cache
3. Validate connectivity
4. Confirm service health

---

## Step 5 — Human Approval

The remediation plan is displayed on the dashboard.

Admin approves remediation before execution.

This demonstrates:

* human-in-the-loop safety
* enterprise governance
* controlled autonomous operations

---

## Step 6 — Remediation Execution

The backend executes remediation commands on the endpoint.

Example:

```powershell
Restart-Service OpenVPNService
ipconfig /flushdns
```

---

## Step 7 — Recovery Validation

The endpoint agent sends updated telemetry.

Agent Builder validates:

* service recovery
* connectivity restoration
* healthy operational state

---

## Step 8 — AI Incident Summary

Agent Builder generates a final incident report.

Example:

```text
Incident Summary

Root Cause:
VPN service unexpectedly stopped after update.

Actions Taken:
- Restarted VPN service
- Flushed DNS cache
- Revalidated connectivity

Result:
VPN connectivity restored successfully.
```

---

# Why This Architecture Matters

This architecture demonstrates:

* Agentic AI workflows
* MCP-based tool usage
* Multi-step reasoning
* Operational orchestration
* Autonomous remediation
* Enterprise AI operations

The project is therefore not simply:

> "a monitoring dashboard"

Instead, it becomes:

> an autonomous operational incident-response agent powered by Google Cloud Agent Builder and Elastic MCP workflows.

---

# Architecture Responsibilities

## Endpoint Agent

Responsible for:

* heartbeat monitoring
* telemetry collection
* service health checks
* command execution
* log streaming

---

## Backend / Telemetry Layer

Responsible for:

* maintaining WebSocket sessions
* storing telemetry
* exposing remediation APIs
* forwarding operational context to Agent Builder
* command dispatching

---

## Elastic + Elastic MCP

Responsible for:

* operational log storage
* telemetry search
* incident retrieval
* observability dashboards
* operational context retrieval

---

## Google Cloud Agent Builder

Responsible for:

* orchestration
* reasoning
* workflow planning
* MCP interactions
* remediation planning
* summary generation

---

## Gemini

Responsible for:

* root-cause analysis
* operational reasoning
* remediation recommendations
* incident summarization

---

# Components

## 1. Endpoint Agent

Responsibilities:

* Heartbeat monitoring
* Collect telemetry
* Stream logs
* Execute commands
* Report health status

Suggested implementation:

* .NET Background Service
* Go daemon/service

Recommended telemetry:

* CPU usage
* Memory usage
* Service status
* VPN connectivity
* Process health
* Error logs

---

## 2. Backend Orchestrator

Responsibilities:

* Maintain device sessions
* Receive telemetry
* Trigger AI analysis
* Manage workflows
* Send remediation commands
* Store incident state

Suggested stack:

* ASP.NET Core
* Go backend
* Redis
* WebSockets

---

## 3. AI Reasoning Layer

Responsibilities:

* Analyze logs
* Correlate telemetry
* Identify root causes
* Suggest remediation
* Generate summaries

Suggested tools:

* Gemini
* Google Cloud AI services

---

## 4. Dashboard

Responsibilities:

* Show live health state
* Display incidents
* Visualize remediation
* Approve actions
* Show summaries

Suggested frontend:

* React
* Tailwind CSS
* Real-time WebSocket updates

---

# Recommended Features

## Essential Features

Build these first:

* Endpoint heartbeat
* Live telemetry
* WebSocket communication
* Log collection
* AI reasoning
* Remediation commands
* Approval workflow
* Incident summary

---

## Nice-To-Have Features

Optional additions:

* Incident timeline
* Deployment awareness
* Historical incidents
* AI confidence score
* Metrics dashboard
* Multi-endpoint support
* Alert prioritization

---

# Suggested Technology Stack

## Backend

* ASP.NET Core
* Go
* Redis
* WebSockets

## Frontend

* React
* Tailwind CSS
* Recharts

## AI

* Gemini
* Google Cloud AI APIs

## Observability

* Elastic (recommended partner track)

---

# Why Elastic Is Recommended

Elastic is likely the strongest partner track because it provides:

* Logs
* Dashboards
* Observability
* Search
* Incident visualization

This naturally complements:

* AI incident analysis
* Root cause identification
* Operational workflows

---

# Important Design Principles

## 1. Keep The Endpoint Alive

Do NOT attempt true offline recovery.

The endpoint should remain reachable.
Only the monitored service/process should fail.

---

## 2. Focus On One Excellent Workflow

Do NOT build:

* Generic AI platform
* Massive automation suite
* Complex distributed systems

Instead:

* Build one polished workflow extremely well.

---

## 3. Optimize For Demo Quality

The demo matters more than scale.

Judges will remember:

> The AI detected the issue and fixed it automatically.

---

# Suggested Final Demo Script

## Opening

"This is an autonomous AI operations agent that monitors endpoint health and automatically remediates operational incidents."

---

## Trigger Failure

Stop a monitored service.

Dashboard turns red.

---

## AI Investigation

Show:

* telemetry
* logs
* AI reasoning

---

## AI Recommendation

AI proposes remediation.

---

## Approval

Admin approves remediation.

---

## Execution

AI executes fix.

Service recovers.

Dashboard returns green.

---

## Incident Summary

AI generates final operational report.

---

# What Makes This Project Strong

This project demonstrates:

* Agentic AI
* Multi-step reasoning
* Autonomous workflows
* Tool usage
* Real-world enterprise value
* Operational automation
* Human-in-the-loop safety
* Technical sophistication

Unlike generic chatbots, this project performs real actions and produces measurable operational outcomes.

---

# Future Expansion Ideas

Potential post-hackathon improvements:

* Multi-agent orchestration
* Security incident response
* Deployment rollback automation
* Fleet-wide monitoring
* Predictive failure analysis
* Autonomous patch management
* Distributed device coordination
* AI governance workflows

---

# Final Recommendation

The best strategy for the hackathon is:

* Keep the scope focused
* Build one polished remediation workflow
* Prioritize demo quality
* Emphasize autonomy and reasoning
* Demonstrate real operational value

The strongest single workflow is likely:

> AI Auto-Healing VPN / Service Incident Response

because it is:

* simple
* realistic
* visually clear
* enterprise-relevant
* technically achievable
* highly demoable
* aligned with judging criteria.



I updated the documentation to include a complete:

* Google Cloud Agent Builder workflow
* Elastic MCP integration flow
* revised compliant architecture
* Agent Builder responsibilities
* operational orchestration sequence
* MCP query examples
* remediation workflow lifecycle
* enterprise reasoning flow

The documentation now aligns much more closely with the hackathon rules and clearly positions:

* Agent Builder as the orchestration brain
* Elastic MCP as the operational context layer
* Gemini as the reasoning engine

instead of making the backend directly call Gemini.
