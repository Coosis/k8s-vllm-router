# Routed vLLM Benchmarks

This directory contains benchmarks where traffic goes through `k8s-vllm-router` before reaching vLLM.

The router discovers vLLM pods through Kubernetes, tracks which backend recently served each prompt prefix, and routes future matching prefixes back toward the warm backend when that backend is healthy and not overloaded. It does not share KV cache between vLLM processes; it preserves locality so vLLM can reuse its own per-process cache.

## Setup

All runs use `Qwen/Qwen2.5-0.5B-Instruct`.

Each test directory contains the Kubernetes manifests and benchmark script for that run. Result JSON files are stored under each run's result directory.

Directory naming:

- `single_...`: one vLLM pod behind the router.
- `3_...`: three vLLM pods behind the router.
- `..._shared`: requests share one of 32 long common prefixes.
- directories without `shared`: requests do not intentionally share a reusable prefix.

## Results

| Run | Pods | Shared Prefixes | TTFT p50 | TTFT p95 | Latency p50 | Latency p95 |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| `single_pod_20260426` | 1 | no | 2074 ms | 4851 ms | 10366 ms | 14441 ms |
| `single_pod_20260426_shared` | 1 | yes | 1089 ms | 4895 ms | 3923 ms | 9241 ms |
| `3_pods_20260426` | 3 | no | 4096 ms | 8299 ms | 10551 ms | 14775 ms |
| `3_pods_20260426_shared` | 3 | yes | 315 ms | 6946 ms | 4097 ms | 12348 ms |

## Dashboard Captures

![3 pods with no common prefix routed](https://coosisv.cc/k8s_vllm_router/20260426/3pods_noshare_routed.png)
![3 pods with shared prefix routed](https://coosisv.cc/k8s_vllm_router/20260426/3pods_shared_routed.png)
![1 pod with no common prefix routed](https://coosisv.cc/k8s_vllm_router/20260426/single_pod_noshare_routed.png)
![1 pod with shared prefix routed](https://coosisv.cc/k8s_vllm_router/20260426/single_pod_shared_routed.png)

## Comparison With Baseline

The key comparison is the three-pod shared-prefix workload, because that is where normal Kubernetes load balancing loses cache locality.

| Scenario | TTFT p50 | Latency p50 |
| --- | ---: | ---: |
| Baseline, 3 pods, shared prefixes | 3161 ms | 9459 ms |
| Routed, 3 pods, shared prefixes | 315 ms | 4097 ms |

With the router, TTFT p50 improves by about 90% and latency p50 improves by about 57% for the three-pod shared-prefix workload.

Within the routed runs, the difference between random-prefix traffic and shared-prefix traffic is also clear. With three pods, TTFT p50 drops from 4096 ms to 315 ms when prefixes are reusable. That is the expected shape if requests with the same prefix are being sent back to the vLLM pod that has the warm KV cache.

## What This Shows

The router is not trying to beat vLLM's cache. It is making Kubernetes routing cache-aware enough that vLLM's existing cache can matter in a multi-pod deployment.

For random-prefix traffic, the router should mostly behave like a load-aware proxy and spread work across available backends. For shared-prefix traffic, it should prefer the backend with the warmest known matching prefix, while still respecting health and pressure.

Useful router metrics to inspect during these runs:

- `router_backend_selected_total{backend}` shows whether traffic is being distributed or intentionally concentrated.
- `router_backend_inflight_requests{backend}` shows current backend pressure.
- `router_prefix_candidate_matches_total{match_length}` shows whether routing found prefix matches.
- `router_prefix_match_warmth_bucket` and `router_prefix_match_age_seconds_bucket` show how fresh and useful those matches were.
- `router_prefix_metadata_evictions_total` shows prefix metadata expiry behavior.

## Caveats

These are local Kubernetes/vLLM benchmark runs, not production GPU serving numbers. The useful signal is the relative behavior between direct Service routing and cache-aware routing under the same workload shape.

The router does not observe vLLM's internal KV cache directly. Cache benefit is inferred from request latency, TTFT, routing decisions, and prefix metadata metrics.
