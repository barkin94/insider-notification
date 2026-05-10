# PRIORITY ROUTER — Processor Service

## Problem
Strict priority ordering starves lower-priority streams under sustained high load. Weighted round-robin gives every priority a guaranteed throughput share.

## Design
A single `PriorityRouter` owns all scheduling. Workers call `Next()` in a tight loop and receive the next message without knowing its origin stream.

## Weights

| Priority | Weight | Share |
|----------|--------|-------|
| high     | 3      | ~50%  |
| normal   | 2      | ~33%  |
| low      | 1      | ~17%  |

Weights are configurable at construction and are a statistical target, not a hard guarantee.

## Algorithm

Pre-expand weights into a repeating slot sequence:
```
weights = {high:3, normal:2, low:1}
slots   = [high, high, high, normal, normal, low]
cursor  = 0   (advances every Next() call)
```

Each `Next()`:
1. `slot = slots[cursor % len(slots)]`, `cursor++`
2. Non-blocking receive from `slot` channel — if available, return it
3. Non-blocking cascade: try high → normal → low — return first available
4. All empty: block on all three channels with 1 s timeout — return message or `(nil, false)`

Step 3 ensures a worker never idles when any queue has work. Fallback-induced ratio deviation is self-correcting because the cursor keeps advancing.

## Interface

```
PriorityRouter

  constructor(high, normal, low: channel<Message>, weights: [3]int)
    panics if any weight < 1

  Next(ctx): (Message, bool)
    (message, true)  — message available
    (nil,     false) — ctx cancelled or 1 s idle timeout
    concurrent-safe; caller owns Ack/Nack
```

## Fallback Table

| Scheduled | high empty | normal empty | low empty | Action       |
|-----------|------------|--------------|-----------|--------------|
| high      | yes        | no           | —         | serve normal |
| high      | yes        | yes          | no        | serve low    |
| normal    | —          | yes          | no        | serve low    |
| normal    | —          | yes          | yes       | block (1 s)  |
| low       | —          | —            | yes       | block (1 s)  |
| any       | yes        | yes          | yes       | block (1 s)  |

## Tests

| Test                | Assertion |
|---------------------|-----------|
| `WeightedRatio`     | 600 pre-loaded messages (300H/200N/100L) consumed in ~3:2:1 ratio (±5%) |
| `FallbackHighEmpty` | high empty → normal slot serves normal |
| `FallbackAllEmpty`  | all empty → Next blocks ~1 s, returns false |
| `ConcurrentSafe`    | 10 goroutines, no race, all messages consumed exactly once |
| `ContextCancel`     | cancelled ctx → Next returns false immediately |
