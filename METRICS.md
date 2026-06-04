# Metrics — Sources & Cadence

This document describes where each UI metric comes from, what determines when it updates, and the total end-to-end lag from a real-world event (workflow started, task queued, Lambda invoked) to the number changing on screen.

---

## Shared Timing Infrastructure

Every metric passes through two shared delays before it renders in the browser:

| Layer | Value | Location |
|---|---|---|
| Server-side cache TTL | **2 seconds** | `demo-app/main.go` → `cache.NewMetricsCache(2 * time.Second)` |
| Frontend poll interval | **every 2 seconds** | `demo-app/frontend/index.html` → `setInterval(fetchMetrics, 2000)` |
| **Worst-case base lag** | **~4 seconds** | Cache expires just after a poll fires; next poll waits the full 2s, reads fresh data that is itself up to 2s stale |

The sparklines hold **30 points × 2 seconds = 60 seconds** of history, which matches the "60s ago" label displayed in the UI.

---

## Per-Metric Details

### Running Workflows

| | |
|---|---|
| **API** | Temporal visibility — `CountWorkflow` |
| **Query** | `TaskQueue="<demo>" AND ExecutionStatus="Running"` |
| **Code** | `demo-app/api/metrics.go` → `fetchTemporalWorkflowCounts` |
| **Source lag** | Temporal visibility is eventually consistent; state transitions propagate in ~1–10 seconds (typically ~1–3s on Temporal Cloud) |
| **Total UI lag** | ~3–14 seconds |

---

### Completed Workflows

| | |
|---|---|
| **API** | Temporal visibility — `CountWorkflow` |
| **Query** | `TaskQueue="<demo>" AND ExecutionStatus="Completed"` |
| **Code** | `demo-app/api/metrics.go` → `fetchTemporalWorkflowCounts` |
| **Source lag** | Same visibility propagation as Running Workflows |
| **Total UI lag** | ~3–14 seconds |

---

### Lambda Invocations

> **Note:** This metric previously used CloudWatch `GetMetricStatistics` (1–3 minute lag, 60-second period granularity). It was replaced with a real-time Temporal poller count.

| | |
|---|---|
| **API** | Temporal — `DescribeTaskQueue` (activity task queue) |
| **Value** | `len(resp.Pollers)` — the number of active activity-task poller connections |
| **Code** | `demo-app/api/metrics.go` → `fetchTaskQueueStats` (activity queue branch) |
| **Why this works** | Each Lambda invocation starts a Temporal worker that registers exactly one activity-task poller. The count is tracked in real time by Temporal Server. |
| **Source lag** | Near-zero — Temporal Server updates poller lists within seconds of a connection opening or closing |
| **Total UI lag** | ~2–6 seconds |

---

### Task Queue Backlog Depth

| | |
|---|---|
| **API** | Temporal — `DescribeTaskQueue` with `ReportStats: true` |
| **Value** | `resp.Stats.ApproximateBacklogCount` summed across workflow + activity task queue types |
| **Code** | `demo-app/api/metrics.go` → `fetchTaskQueueStats` |
| **Source lag** | Real-time read from Temporal Server's task queue management layer (not the visibility/search subsystem) |
| **Total UI lag** | ~2–6 seconds |

---

### Sync Match Rate

| | |
|---|---|
| **API** | Temporal — same two `DescribeTaskQueue` calls as Backlog Depth |
| **Value** | Derived: `(1 - (TasksAddRate - TasksDispatchRate) / TasksAddRate) × 100` |
| **Code** | `demo-app/api/metrics.go` → `fetchTaskQueueStats` |
| **Sentinel** | Returns `-1` (displayed as "—") when `TasksAddRate == 0` — queue is idle, no meaningful rate to show |
| **Source lag** | `TasksAddRate` and `TasksDispatchRate` are exponentially-weighted moving averages computed internally by Temporal Server. They respond within seconds but smooth over a window of ~seconds to ~30 seconds, so the rate lags actual task dispatch slightly and recovers gradually after a burst ends. |
| **Total UI lag** | ~4–36 seconds |

---

## Summary Table

| Metric | API | Freshness | Total UI Lag |
|---|---|---|---|
| Running Workflows | Temporal visibility (`CountWorkflow`) | Eventually consistent | ~3–14s |
| Completed Workflows | Temporal visibility (`CountWorkflow`) | Eventually consistent | ~3–14s |
| Lambda Invocations | Temporal `DescribeTaskQueue` (poller count) | Real-time | ~2–6s |
| Backlog Depth | Temporal `DescribeTaskQueue` (stats) | Real-time | ~2–6s |
| Sync Match Rate | Temporal `DescribeTaskQueue` (rate EMA) | Smoothed real-time | ~4–36s |

All five metrics are sourced exclusively from Temporal — no AWS/CloudWatch dependency. Three of the five (Lambda invocations, backlog depth, sync match rate) update in the same **2–6 second** freshness window, making the core causal story visible on screen in near real time:

```
backlog ↑  →  sync match rate ↓  →  Lambda workers ↑  →  backlog ↓  →  workflows complete
```
