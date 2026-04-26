# Baseline vLLM Benchmarks

This directory contains the control benchmarks for plain vLLM behind Kubernetes, without the router.

The goal is to establish the problem the router is meant to solve:

- vLLM prefix caching is valuable when repeated prompts land on the same process.
- A normal Kubernetes Service gives useful multi-pod parallelism.
- That same Service does not preserve prefix-cache locality, because repeated prefixes are spread across different vLLM pods.

## Setup

All runs use `Qwen/Qwen2.5-0.5B-Instruct`.

Each test directory contains the Kubernetes manifests and the benchmark script used for that run. Result JSON files are stored under each run's result directory.

Directory naming:

- `single_...`: one vLLM pod.
- `3_...`: three vLLM pods behind a Kubernetes Service.
- `..._shared`: requests share one of 32 long common prefixes.
- directories without `shared`: requests do not intentionally share a reusable prefix.

The shared-prefix workload is the important case for this project. It represents chat, agent, RAG, or system-prompt-heavy traffic where many requests reuse a long prompt prefix and should benefit from vLLM's KV cache.

## Results

| Run | Pods | Shared Prefixes | TTFT p50 | TTFT p95 | Latency p50 | Latency p95 |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| `single_pod_20260423` | 1 | no | 3852 ms | 16864 ms | 32642 ms | 57357 ms |
| `single_pod_20260423_shared` | 1 | yes | 1123 ms | 4956 ms | 4007 ms | 9461 ms |
| `3_pods_20260423` | 3 | no | 3301 ms | 8171 ms | 9462 ms | 25526 ms |
| `3_pods_20260423_shared` | 3 | yes | 3161 ms | 9098 ms | 9459 ms | 16334 ms |

## Interpretation

The single-pod comparison proves the cache effect. When all shared-prefix traffic lands on one vLLM process, TTFT p50 drops from 3852 ms to 1123 ms and latency p50 drops from 32642 ms to 4007 ms.

The three-pod random-prefix run shows the value of Kubernetes scaling. Compared with the single-pod random-prefix run, latency p50 improves from 32642 ms to 9462 ms because requests can execute across multiple vLLM pods.

The failure mode appears in the three-pod shared-prefix run. Shared-prefix traffic should be cache-friendly, but TTFT p50 is 3161 ms, which is much closer to the three-pod random-prefix run than the single-pod shared-prefix run. The reusable prefixes are being split across pods, so each pod only sees part of the repetition and the KV cache is less effective.

## Dashboard Captures

![3 pods with no common prefix](https://coosisv.cc/k8s_vllm_router/20260426/3pods_noshare.png)
![3 pods with shared prefix](https://coosisv.cc/k8s_vllm_router/20260426/3pods_shared.png)
![1 pod with no common prefix](https://coosisv.cc/k8s_vllm_router/20260426/single_pod_noshare.png)
![1 pod with shared prefix](https://coosisv.cc/k8s_vllm_router/20260426/single_pod_shared.png)

## Conclusion

Plain Kubernetes gives health management, rollout behavior, and parallelism, but it does not know which vLLM pod has a warm prefix cache. The router exists to add that missing cache-affinity layer while still running vLLM as normal Kubernetes pods.
