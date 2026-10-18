---
id: observability
title: Observability
---

Set `Config.Observe` to collect phase timings without giving a metrics backend
access to query contents:

```go
config.Observe = func(ctx context.Context, event query.Observation) {
    // Pass fixed endpoint/phase/error-code labels to your metrics backend.
    // Record event.Duration.Seconds() in a histogram.
    // An empty event.ErrorCode means success.
}
```

`Engine.List` and `Session.List` emit these phases in order:

| Phase | Work included |
| --- | --- |
| `PhaseParse` | Decode and parse the request using endpoint limits |
| `PhaseValidate` | Bind the query to policy, select default sort, validate cursor |
| `PhaseCount` | Prepare and execute an explicitly requested count, including search |
| `PhaseData` | Prepare and execute the data query, model hooks, and next cursor |

A failure emits the current phase's event with the stable error code and stops
later phases. Count is absent unless requested; data is absent for `limit=0`.
`Parse`, `Apply`, and `ValidateConfig` do not emit List events. Time spent in
the callback itself is excluded from that event's duration. A nil callback
avoids clock reads.

The callback is synchronous and receives the caller's context, including
cancellation. It must be concurrency-safe, return promptly, and not panic.
Gotq does not start goroutines, maintain queues, or recover observer panics.
Use your application's bounded telemetry pipeline if exporting needs I/O.

`Observation` contains only `Phase`, `Duration`, and `ErrorCode`. It never
contains SQL, raw errors, filter/search text, cursor keys, or tenant IDs.
Use fixed endpoint labels instead of URLs or request values to keep metric
cardinality bounded. Although the callback receives the application's context
for trace correlation, avoid exporting identity or sensitive context values.

No metrics, tracing, or HTTP framework dependency is added to the library.
Applications can connect this hook to their existing telemetry provider.
