# PulseOps AI Phase 3 Detailed Plan

Phase: 3 — Backend telemetry ingestion and live device state  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Make the backend remember the latest state of every endpoint and push live updates to the dashboard so the operator sees a real, auto-refreshing health view without polling.

Phase 2 proved the agent can send telemetry. Phase 3 is about what the backend *does* with that telemetry:

- keep an up-to-date picture of each device
- expose that picture over HTTP so the dashboard can GET it
- broadcast every state change over WebSocket so the dashboard reacts in real time

At the end of Phase 3:
- the backend holds the latest telemetry snapshot for every device that has ever checked in
- `GET /devices` and `GET /devices/{deviceId}` return JSON state
- a WebSocket endpoint pushes a state update message to all connected clients whenever a new telemetry payload arrives
- the React dashboard shows a live Endpoint Health card that reflects the running/stopped state of the monitored service without a manual refresh
- a healthy baseline stays visually stable for at least 60 seconds (acceptance criterion from `PHASE_ACCEPTANCE_CRITERIA.md`)

Phase 4 (incident detection) will read the same in-memory store. Keep it clean.

---

## 2) Rule-aware constraints for Phase 3

1. **Required stack alignment**
   - No external message broker (Redis, Kafka) is needed yet. In-memory is sufficient and faster to build.
   - Keep the WebSocket contract generic so the frontend can be extended in Phase 4 without re-wiring.
   - Do not introduce non-permitted AI/cloud dependencies at this phase.

2. **Functional submission requirement**
   - The dashboard must render live data from the real backend, not from a mock.
   - Every step must be reproducible from the README/scripts.

3. **Security baseline**
   - Validate the `Origin` header on WebSocket upgrades and reject unexpected origins.
   - Keep CORS restricted to the allowed frontend origin only.
   - Do not expose raw Go `panic` traces in HTTP responses.

4. **Demo readiness**
   - The WebSocket reconnect behavior must be graceful: if the backend restarts the frontend should re-establish the connection automatically.
   - The dashboard must visually distinguish `running`, `stopped`, and `unknown` states with color coding so a judge can see the health change immediately.

---

## 3) Phase 3 definition of done

Phase 3 is complete only when all are true:

1. `GET /devices` returns JSON array of all device states seen since backend start.
2. `GET /devices/{deviceId}` returns the latest state for that device, or 404.
3. POSTing new telemetry updates the in-memory state and is reflected in the GET responses immediately.
4. `GET /ws` upgrades to a WebSocket connection for frontend clients.
5. Every telemetry POST causes the updated device state to be broadcast to all open WebSocket connections.
6. The React dashboard connects to the WebSocket on load.
7. The Endpoint Health card renders real service status, CPU, memory, and last-seen timestamp.
8. The card CSS reflects `running` (green), `stopped` (red), and `unknown` (grey) states.
9. Disconnecting and reconnecting the WebSocket restores the live feed without a page reload.
10. The acceptance criteria for Phase 3 in `PHASE_ACCEPTANCE_CRITERIA.md` pass.

---

## 4) Work breakdown structure

### 4.1 In-memory device state store

**Goal:** give the backend a single source of truth for current endpoint state that is safe for concurrent reads and writes.

**Tasks:**
1. Create `backend/internal/store/device_store.go`.
2. Define a `DeviceState` struct that mirrors the telemetry payload fields plus metadata:
   ```go
   type DeviceState struct {
       DeviceID         string    `json:"deviceId"`
       Timestamp        string    `json:"timestamp"`
       ServiceName      string    `json:"serviceName"`
       ServiceStatus    string    `json:"serviceStatus"`
       NetworkReachable bool      `json:"networkReachable"`
       CPUUsage         float64   `json:"cpuUsage"`
       MemoryUsage      float64   `json:"memoryUsage"`
       RecentLogs       []string  `json:"recentLogs"`
       Heartbeat        bool      `json:"heartbeat"`
       LastSeenAt       time.Time `json:"lastSeenAt"`
   }
   ```
3. Define a `DeviceStore` struct holding `map[string]*DeviceState` and a `sync.RWMutex`.
4. Implement `NewDeviceStore() *DeviceStore`.
5. Implement `Upsert(state DeviceState)` — write-locks, sets or replaces the entry for `state.DeviceID`, sets `LastSeenAt = time.Now().UTC()`.
6. Implement `Get(deviceID string) (*DeviceState, bool)` — read-locks, returns a copy (not a pointer to the map value) to prevent data races.
7. Implement `List() []DeviceState` — read-locks, returns a stable slice copy sorted by `DeviceID` for deterministic API output.

**Why a copy on Get/List:**  
Never return a pointer to an internal map value. Callers that hold the pointer after the lock is released create a data race. Always copy.

**Output:**  
A thread-safe store that Phase 4 will also use for incident detection logic.

---

### 4.2 GET device state API endpoints

**Goal:** let the frontend and any debugging tool read current endpoint state over plain HTTP.

**Tasks:**
1. Create `backend/internal/api/devices.go`.
2. Implement `DevicesHandler(store *store.DeviceStore) http.HandlerFunc` for `GET /devices`.
   - Returns `200` with a JSON array of all `DeviceState` entries (may be empty `[]`).
   - Responds with `405 Method Not Allowed` for non-GET methods.
3. Implement `DeviceByIDHandler(store *store.DeviceStore) http.HandlerFunc` for `GET /devices/{deviceId}`.
   - Extracts the device ID from the URL path (use `strings.TrimPrefix` or `http.ServeMux` path matching).
   - Returns `200` with the device state JSON if found.
   - Returns `404` with a JSON error body `{"error": "device not found"}` if missing.
   - Responds with `405` for non-GET methods.
4. Register both routes in `main.go`:
   ```go
   mux.HandleFunc("/devices", devicesHandler)
   mux.HandleFunc("/devices/", deviceByIDHandler)  // trailing slash for sub-path
   ```

**Response shape for a single device:**
```json
{
  "deviceId": "LAPTOP-22",
  "timestamp": "2026-05-23T10:30:00Z",
  "serviceName": "OpenVPNService",
  "serviceStatus": "running",
  "networkReachable": true,
  "cpuUsage": 12.4,
  "memoryUsage": 48.1,
  "recentLogs": [],
  "heartbeat": true,
  "lastSeenAt": "2026-05-23T10:30:02.114Z"
}
```

**Output:**  
Stable HTTP endpoints that can be `curl`-tested independently of the WebSocket.

---

### 4.3 Wire telemetry handler to update state store

**Goal:** make the existing `POST /telemetry` handler write into the store on every accepted payload.

**Tasks:**
1. Pass `*store.DeviceStore` into `telemetryHandler` (convert it to a closure or method receiver).
2. After successful validation, call `store.Upsert(DeviceState{...})` before writing the HTTP `202` response.
3. Map every field from `TelemetryPayload` to `DeviceState` directly (same fields, different struct).
4. Keep the existing log line — just add one more log field: `"state_updated=true"`.
5. Do not return an error if the upsert fails (it cannot panic; it is pure in-memory).

**Output:**  
Every telemetry POST transparently updates the store. The GET endpoints immediately reflect the new state.

---

### 4.4 WebSocket hub and broadcast

**Goal:** push a device state update to every connected browser tab the moment new telemetry arrives.

**Why WebSocket over polling:**  
The acceptance criterion says "dashboard updates live without manual refresh." Polling at one-second intervals would work but creates unnecessary churn and looks worse in a demo.

#### 4.4.1 Hub design

Create `backend/internal/ws/hub.go`.

The hub manages a set of active client connections and a channel for outbound messages:

```go
type Hub struct {
    clients    map[*Client]struct{}
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
    mu         sync.Mutex
}

type Client struct {
    hub  *Hub
    conn *websocket.Conn
    send chan []byte
}
```

Implement `NewHub() *Hub` and `func (h *Hub) Run()` which is started as a goroutine in `main.go`. The `Run` loop:
- on `register`: add client to map
- on `unregister`: remove client, close its `send` channel, and close the underlying connection
- on `broadcast`: range over clients and non-blocking send to each client's `send` channel; drop slow clients (close and unregister if the channel is full)

#### 4.4.2 Client write pump

Each client runs a `writePump` goroutine that reads from `client.send` and writes to the WebSocket connection. Set a write deadline on every send. On error, call `hub.unregister <- client`.

#### 4.4.3 Client read pump

Each client runs a `readPump` goroutine that reads from the WebSocket. For the MVP the frontend will not send messages, so the read pump only handles close frames and pings. On any read error, call `hub.unregister <- client`.

#### 4.4.4 WebSocket upgrade handler

Create `backend/internal/ws/handler.go`.

```go
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
    // validate origin
    // upgrade connection
    // create Client
    // register with hub
    // start writePump and readPump goroutines
}
```

Add `gorilla/websocket` as a dependency:
```
go get github.com/gorilla/websocket
```

Validate the `Origin` header in the upgrader's `CheckOrigin` function. Accept only origins matching `CORS_ALLOWED_ORIGIN` env var (default `http://localhost:5173` for the Vite dev server).

Register the route in `main.go`:
```go
mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    ws.ServeWs(hub, w, r)
})
```

#### 4.4.5 Broadcast on telemetry receipt

In `telemetryHandler`, after calling `store.Upsert(...)`:
1. Call `store.Get(payload.DeviceID)` to retrieve the freshly updated state.
2. Serialize the state to JSON.
3. Send to `hub.broadcast <- stateJSON` in a non-blocking select (drop if hub is not running yet).

**Output:**  
Every browser tab connected to `/ws` receives a JSON device state object within one telemetry interval of a state change.

---

### 4.5 CORS and backend configuration

**Goal:** let the Vite dev server and the production frontend origin connect to the backend.

**Tasks:**
1. Add a CORS middleware function in `backend/internal/api/middleware.go`:
   - Reads `CORS_ALLOWED_ORIGIN` env var.
   - Sets `Access-Control-Allow-Origin`, `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`.
   - Handles `OPTIONS` preflight with `204`.
2. Wrap all handlers with this middleware in `main.go`.
3. Add `CORS_ALLOWED_ORIGIN` to the documented environment variables.
4. Default `CORS_ALLOWED_ORIGIN` to `http://localhost:5173` when unset.

**Important:** never set `Access-Control-Allow-Origin: *` in production. Use the env var.

**Output:**  
Frontend can make `fetch()` and WebSocket calls to the backend without CORS errors during development.

---

### 4.6 Frontend: TypeScript types for live device state

**Goal:** define the exact shape the frontend expects from the backend so the compiler catches field mismatches.

**Tasks:**
1. Add to `frontend/src/types/dashboard.ts`:

```typescript
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
```

2. Expand `DashboardCard` status field:

```typescript
export type CardStatus = 'placeholder' | 'healthy' | 'degraded' | 'stopped' | 'unknown'

export type DashboardCard = {
  title: string
  status: CardStatus
  description: string
}
```

**Output:**  
Compile-time safety for all components that render device state data.

---

### 4.7 useDeviceState hook

**Goal:** give any React component a simple, auto-updating device state value with no manual refresh.

**File:** `frontend/src/hooks/useDeviceState.ts`

**Tasks:**
1. Accept `deviceId: string` as a parameter.
2. Open a WebSocket connection to `${wsBaseUrl}/ws` on mount where `wsBaseUrl` is derived from `VITE_API_BASE_URL` by replacing the `http` scheme with `ws`.
3. On each incoming message, parse the JSON and check `msg.deviceId === deviceId`; if it matches, update the state.
4. On WebSocket `close` or `error`, schedule a reconnect with a 3-second delay using `setTimeout`. Clear the timer on unmount.
5. On unmount, close the WebSocket cleanly (readyState check before close).
6. Return `{ deviceState: DeviceState | null, connected: boolean }`.

**Fallback for when WebSocket is not yet available:**  
If the first WebSocket connection fails (backend not running), fall back to a one-time `GET /devices/{deviceId}` fetch so the UI still renders something useful.

**Env helper** — add to `frontend/src/hooks/useApiBaseUrl.ts`:
```typescript
export function useWsBaseUrl(): string {
  const apiBase = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'
  return apiBase.replace(/^http/, 'ws')
}
```

**Output:**  
Any component can import `useDeviceState` and get a live-updating device state with one line.

---

### 4.8 Update StatusCard and DashboardPage

**Goal:** replace the placeholder cards with real live-data components.

#### 4.8.1 StatusCard

Update `frontend/src/components/StatusCard.tsx`:

1. Accept an optional `deviceState?: DeviceState` prop alongside the existing `card` prop.
2. When `deviceState` is provided, render:
   - Service name and status badge
   - CPU and memory usage bars or percentages
   - Last-seen timestamp formatted as `HH:MM:SS UTC`
   - Recent logs list (last 3 entries max, truncated)
3. Apply a CSS class to the card root based on `serviceStatus`:
   - `running` → `status-running` (green left border)
   - `stopped` → `status-stopped` (red left border)
   - `degraded` → `status-degraded` (orange left border)
   - `unknown` or placeholder → `status-unknown` (grey left border)

#### 4.8.2 DashboardPage

Update `frontend/src/pages/DashboardPage.tsx`:

1. Call `useDeviceState(AGENT_DEVICE_ID)` where `AGENT_DEVICE_ID` comes from `import.meta.env.VITE_AGENT_DEVICE_ID || 'LAPTOP-22'`.
2. Pass the live `deviceState` into the Endpoint Health `StatusCard`.
3. Show a `connected` / `reconnecting` badge in the header based on `connected` from the hook.
4. Keep the Incident Timeline, AI Investigation, and Remediation Approval cards as placeholders — they are built in later phases.

**Output:**  
The dashboard Endpoint Health card renders real telemetry data and turns red when the monitored service is stopped.

---

### 4.9 CSS health state styles

**Goal:** make status changes impossible to miss visually.

**Tasks:**
1. Add to `frontend/src/App.css` or a new `frontend/src/components/StatusCard.css`:

```css
.status-card.status-running  { border-left: 4px solid #22c55e; }
.status-card.status-stopped  { border-left: 4px solid #ef4444; }
.status-card.status-degraded { border-left: 4px solid #f97316; }
.status-card.status-unknown  { border-left: 4px solid #6b7280; }

.badge-running  { background: #dcfce7; color: #15803d; }
.badge-stopped  { background: #fee2e2; color: #b91c1c; }
.badge-degraded { background: #ffedd5; color: #c2410c; }
.badge-unknown  { background: #f3f4f6; color: #374151; }
```

2. Apply the dynamic class in `StatusCard.tsx`:
```tsx
<article className={`status-card status-${card.status}`} ...>
```

**Output:**  
Green/red/orange/grey border instantly communicates health state at a glance — critical for demo judging.

---

### 4.10 Environment variables update

**Goal:** document every new variable introduced in Phase 3.

Add to the project `.env.example` files:

**Backend (`backend/.env.example`):**
```
BACKEND_PORT=8080
CORS_ALLOWED_ORIGIN=http://localhost:5173
```

**Frontend (`frontend/.env.example`):**
```
VITE_API_BASE_URL=http://localhost:8080
VITE_AGENT_DEVICE_ID=LAPTOP-22
```

Both env files already exist from Phase 1/2 — only add the new keys.

---

### 4.11 Smoke-check path

**Goal:** provide a deterministic end-to-end test that confirms Phase 3 is working.

**Manual steps:**
1. Start backend: `scripts/run-backend.ps1`
2. Start agent: `scripts/run-agent.ps1`
3. Start frontend: `scripts/run-frontend.ps1`
4. Open browser at `http://localhost:5173`
5. Confirm the Endpoint Health card shows `running` with green border and real CPU/memory values.
6. Confirm "connected" badge is visible in the dashboard header.
7. Open a second browser tab — confirm both tabs update simultaneously when a new telemetry arrives.
8. Stop the monitored service manually (see Phase 2 smoke-check procedure).
9. Within one heartbeat interval (≤10 seconds) confirm the card turns red and `serviceStatus: stopped` is visible.
10. Restart the service. Confirm the card returns to green within one heartbeat interval.

**curl verification (without frontend):**
```powershell
# After at least one telemetry from the agent:
Invoke-RestMethod http://localhost:8080/devices
Invoke-RestMethod http://localhost:8080/devices/LAPTOP-22
```

Both should return JSON device state, not an empty object.

**Output:**  
A repeatable, judge-demonstrable live dashboard that proves the end-to-end telemetry flow.

---

## 5) Detailed implementation order

Work in this sequence to keep the backend and frontend always in a runnable state:

1. Create `backend/internal/store/device_store.go` — the store has no dependencies, build it first.
2. Add `gorilla/websocket` dependency with `go get`.
3. Create `backend/internal/ws/hub.go` and `hub_test.go` for hub register/unregister/broadcast.
4. Create `backend/internal/ws/handler.go` for the WebSocket upgrade handler.
5. Create `backend/internal/api/devices.go` for the GET endpoints.
6. Create `backend/internal/api/middleware.go` for CORS.
7. Update `backend/cmd/server/main.go` to wire the store, hub, CORS, and all new routes.
8. Update `telemetryHandler` in `main.go` to call `store.Upsert` and `hub.broadcast`.
9. Verify backend compiles and all routes respond correctly with `curl`.
10. Update `frontend/src/types/dashboard.ts` with `DeviceState` and `CardStatus` types.
11. Add `useWsBaseUrl` to `frontend/src/hooks/useApiBaseUrl.ts`.
12. Create `frontend/src/hooks/useDeviceState.ts`.
13. Update `frontend/src/components/StatusCard.tsx` to accept and render `DeviceState`.
14. Update `frontend/src/pages/DashboardPage.tsx` to call the hook and pass live data.
15. Add CSS health state styles.
16. Run the full smoke-check sequence.
17. Update `PHASE_ACCEPTANCE_CRITERIA.md` checkboxes for Phase 3.

**Why this order:**
- backend store and hub have no frontend dependency; ship them first
- the GET endpoints let you validate the store works before WebSocket is wired
- frontend types come before hook implementation to catch type errors early
- the StatusCard update is last because it depends on the hook being stable

---

## 6) Key implementation details

### 6.1 Avoiding data races in the hub

The hub's `Run()` goroutine is the only place that reads or writes the `clients` map. All other goroutines communicate with it exclusively through the `register`, `unregister`, and `broadcast` channels. Do not lock the `clients` map directly from outside `Run()`.

### 6.2 Slow client protection

If a client's `send` channel is full (buffer of 256 bytes is a safe default), the hub should unregister and close it rather than blocking `broadcast`. A blocked broadcast would delay all other clients and stall the telemetry loop.

```go
select {
case client.send <- message:
default:
    close(client.send)
    delete(h.clients, client)
}
```

### 6.3 WebSocket message shape

Broadcast messages are the full `DeviceState` JSON object. The frontend identifies which device the update belongs to via `msg.deviceId`. Keep the shape identical to the GET `/devices/{id}` response — one schema, two delivery paths.

### 6.4 React WebSocket lifecycle

In `useDeviceState`, use a `useRef` for the WebSocket instance so closing and reconnecting does not trigger a re-render loop. Only update `connected` state (a `useState`) when the connection state actually changes.

### 6.5 GET /devices route disambiguation

`http.ServeMux` does not support path parameters natively. Use the pattern `/devices/` (with trailing slash) to match any sub-path. In the handler, extract the device ID with:

```go
deviceID := strings.TrimPrefix(r.URL.Path, "/devices/")
```

Reject empty `deviceID` (bare `/devices/` request) with `404`.

---

## 7) Environment variables reference

| Variable | Component | Default | Purpose |
|---|---|---|---|
| `BACKEND_PORT` | backend | `8080` | HTTP listen port |
| `CORS_ALLOWED_ORIGIN` | backend | `http://localhost:5173` | Allowed frontend origin for CORS and WS upgrade |
| `VITE_API_BASE_URL` | frontend | `http://localhost:8080` | Base URL for REST calls |
| `VITE_AGENT_DEVICE_ID` | frontend | `LAPTOP-22` | Which device ID to subscribe to in the health card |

---

## 8) New files to create

| File | Purpose |
|---|---|
| `backend/internal/store/device_store.go` | Thread-safe in-memory device state map |
| `backend/internal/ws/hub.go` | WebSocket hub — client registry and broadcast loop |
| `backend/internal/ws/handler.go` | HTTP → WebSocket upgrade handler |
| `backend/internal/api/devices.go` | GET /devices and GET /devices/{id} handlers |
| `backend/internal/api/middleware.go` | CORS middleware |
| `frontend/src/hooks/useDeviceState.ts` | WebSocket hook for live device state |

## 9) Files to modify

| File | Change |
|---|---|
| `backend/cmd/server/main.go` | Wire store, hub, new routes, CORS, and update `telemetryHandler` signature |
| `frontend/src/types/dashboard.ts` | Add `DeviceState`, `ServiceStatus`, `CardStatus` |
| `frontend/src/hooks/useApiBaseUrl.ts` | Add `useWsBaseUrl()` |
| `frontend/src/components/StatusCard.tsx` | Render live `DeviceState` props and apply status CSS class |
| `frontend/src/pages/DashboardPage.tsx` | Call `useDeviceState`, pass data to health card, show connection badge |
| `frontend/src/App.css` | Add health-state color classes |
| `backend/.env.example` | Add `CORS_ALLOWED_ORIGIN` |
| `frontend/.env.example` | Add `VITE_AGENT_DEVICE_ID` |

---

## 10) Phase 3 acceptance gate

Pass when all three are true (from `PHASE_ACCEPTANCE_CRITERIA.md`):

- [ ] Backend stores latest endpoint state — confirmed by `GET /devices/{deviceId}` returning real telemetry data.
- [ ] Dashboard updates live without manual refresh — confirmed by service-stop turning the card red within 10 seconds.
- [ ] Healthy baseline is visually stable for at least 60 seconds — confirmed by watching the dashboard with the service running and no incidents.
