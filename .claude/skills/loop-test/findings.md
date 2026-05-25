# loop-test findings

Empirical observations from `/loop` experiments. Newest at top.

---

### 2026-05-25 · Experiment 1 · multi-tick + clamp + self-termination

**Loop prompt:**
```
/loop Append the current ISO timestamp (date -u +"%Y-%m-%dT%H:%M:%SZ") as a new line to .lets/goal-test/loop-pings.txt. After appending, count lines in the file. If file has 3 or more lines, paste the full contents back into the response and STOP — do NOT call ScheduleWakeup. Otherwise call ScheduleWakeup with delaySeconds=30 (test clamp), prompt=<this entire /loop prompt verbatim>, reason="loop-test tick N".
```

**Outcome:** clean self-termination after 3 ticks.

**Timing observed:**
| Tick | Append timestamp (UTC) | Tool-reported next-delay | Actual gap to next tick |
|---|---|---|---|
| 1 | 16:19:45Z | clamped 60s, scheduled "in 65s" | 88s |
| 2 | 16:21:13Z | clamped 60s, scheduled "in 101s" | 116s |
| 3 | 16:23:09Z | n/a — no ScheduleWakeup, loop ended | — |

**Key empirical findings:**

1. **`delaySeconds` clamp works as documented.** Requested `30s` → tool returned `"clamped to 60s from your requested value"`. Binary research (`[60, 3600]`) confirmed in practice.
2. **Actual wakeup is rounded UP to next clean minute boundary.** Tool-reported "scheduled for HH:MM:00 (in Ns)" — N varies 60-119s depending on `now()` within the current minute. Binary cron expression is `MM HH * * *` (minute-only granularity); confirmed. **Implication:** `delaySeconds=60` does NOT mean "exactly 60s from now" — it means "at least 60s, then next clean minute."
3. **Dispatch latency:** tick fires ~9-23s after scheduled time. Not exact-second; harness re-invocation has overhead.
4. **Self-termination works cleanly.** Model honored explicit instruction `"STOP — do NOT call ScheduleWakeup"`. Loop ended without zombie tick. No special teardown needed.
5. **Cost per tick.** Each tick is a fresh model turn. The full `/loop` skill markdown (~3KB) is re-injected into prompt every tick. Cache TTL 5min → 60s ticks keep skill content warm; ticks >300s pay cache miss.
6. **State passing.** No automatic state mechanism. Persistent state must be either in (a) the prompt verbatim, or (b) the file system. Each tick is otherwise stateless.
7. **The /loop spec lives inside the slash-command markdown itself,** not as a skill called by the model. The model parses the spec's "Dynamic mode" section directly and follows it. Means: customizing /loop behavior via plugin would require either a wrapper command or a project-level hook.

**Surprises:**
- Tool-reported delay ("in Ns") is **not the same as the requested clamped value** when minute-boundary rounding fires. Worth surfacing in our SKILL.md as a gotcha — users designing precise polling cadences will be off by up to 60s.
- The /loop skill's instruction to prefix prompt with `/loop ` on ScheduleWakeup is the only thing keeping the loop alive on re-invocation. Without it, the next tick would be a one-off prompt with no /loop semantics. **Hard error mode:** if a future skill version changed prefix conventions, all in-flight loops would orphan silently.

**LETS workflow fit:**
- ✓ Good for: anything where you can pre-encode the loop body and stop-condition in one self-contained prompt (e.g. polling external state, multi-step incremental work with file-based state).
- ✗ Bad for: any flow where "what to do next" depends on conversation history. The model wakes up cold each tick — only file system + prompt-verbatim survive.
- ✗ Bad for: anything requiring more frequent than ~1min polling (clamp + minute-boundary rounding makes <60s impossible).

**Promote to plugin?** Not yet. Useful primitive but the `/lets:*` workflow currently has no use case that fits the "stateless ticks + file-system memory" pattern better than `/goal` does. Re-evaluate after `/goal` Template A/B/C empirical runs.

**Cost:** 3 ticks × (1 Bash + 1 ScheduleWakeup or text) ≈ ~1.5K tokens total per loop run. Trivial.
