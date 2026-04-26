# k8s-vLLM Router

[![CI](https://github.com/Coosis/k8s-vllm-router/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Coosis/k8s-vllm-router/actions/workflows/ci.yml)

`k8s-vllm-router` is a Kubernetes-native OpenAI-compatible router for vLLM.

It keeps the normal Kubernetes operating model: pods, Services, health checks, rolling updates, and EndpointSlice discovery. The part Kubernetes does not know is vLLM KV-cache locality. If requests with the same long prefix are randomly balanced across pods, each vLLM process warms its own partial cache and the shared-prefix benefit is diluted.

This router adds a cache-aware routing layer. It fingerprints prompt prefixes, remembers which backend recently served each prefix, and routes later matching requests back toward the warm backend when it is healthy and not overloaded.

## Quickstart

Prerequisites:

- A Kubernetes cluster.
- vLLM pods running in the same namespace as the router.
- A Kubernetes Service in front of vLLM. The default router manifest expects this Service to be named `vllm-service`.
- vLLM health checks available at `/health`.

Deploy the router:

```sh
kubectl apply -f deploy/kubernetes/router.yaml
```

Wait for the router to become ready:

```sh
kubectl rollout status deploy/k8s-vllm-router
kubectl get pods -l app=k8s-vllm-router
```

Forward the router Service:

```sh
kubectl port-forward svc/k8s-vllm-router 8080:8080
```

Send an OpenAI-compatible request through the router:

```sh
curl -s http://localhost:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "Qwen/Qwen2.5-0.5B-Instruct",
    "messages": [
      {
        "role": "user",
        "content": "You are a concise assistant. Explain what KV cache locality means for vLLM."
      }
    ],
    "max_tokens": 128,
    "temperature": 0
  }'
```

If your vLLM Service uses a different name, update `DISCOVERY_SERVICE` in the router ConfigMap:

```yaml
DISCOVERY_SERVICE: your-vllm-service-name
```

Useful router endpoints:

- `POST /v1/chat/completions`: OpenAI-compatible chat completions proxy.
- `POST /v1/completions`: OpenAI-compatible completions proxy.
- `GET /healthz`: process liveness.
- `GET /readyz`: returns ready when at least one backend is healthy.
- `GET /metrics`: Prometheus metrics.

## Kubernetes Usage

The router discovers vLLM pods from Kubernetes EndpointSlices. The expected setup is:

- vLLM runs as normal pods behind a Kubernetes Service.
- The router runs in the same namespace.
- `DISCOVERY_SERVICE` points at the vLLM Service name.
- The router Service exposes the OpenAI-compatible endpoints.

The published image is `docker.io/coosis/k8s-vllm-router:latest`.

The included manifest exposes the router as a NodePort on `31080`. If NodePort access is available in your cluster, requests can also be sent to:

```sh
http://<node-ip>:31080/v1/chat/completions
```

## Configuration

The router is configured with environment variables. Values below match the Kubernetes manifest where it sets them; otherwise they describe the normal default.

| Variable | Value | Purpose |
| --- | --- | --- |
| `ROUTER_ADDR` | `:8080` | HTTP listen address. |
| `ROUTER_POLICY` | `cache_aware` | Routing policy. |
| `BACKENDS` | unset | Static backends, as `id=url,id=url`. Ignored when dynamic discovery is configured. |
| `DISCOVERY_SERVICE` | `vllm-service` | Kubernetes Service name to discover through EndpointSlices. |
| `POD_NAMESPACE` | pod namespace | Namespace for EndpointSlice discovery. Set from the Downward API in the manifest. |
| `DISCOVERY_SCHEME` | `http` | URL scheme for discovered backends. |
| `DISCOVERY_PORT_NAME` | empty | Optional named Service port to select. Empty means use the EndpointSlice port. |
| `DISCOVERY_INTERVAL` | `5s` | EndpointSlice polling interval. |
| `HEALTH_CHECK_INTERVAL` | `5s` | Backend health polling interval. |
| `BACKEND_HEALTH_TIMEOUT` | `5s` | Per-backend health check timeout. |
| `BACKEND_HEALTH_FAILURE_THRESHOLD` | `3` | Consecutive failures before marking a backend unhealthy. |
| `BACKEND_HEALTH_PATH` | `/health` | Backend health path for vLLM. |
| `PREFIX_MAX_AGE` | `180` | Hard cap for prefix metadata age, in seconds. |
| `EXPIRY_POLICY` | `decay` | Prefix metadata expiry policy. |
| `DECAY_POLICY` | `exponential` | Prefix warmth decay policy. |
| `DECAY_HALF_LIFE` | `90s` | Half-life for exponential prefix warmth decay. |

## How Routing Works

For each request, the router extracts the prompt text and computes fingerprints for several prefix lengths. The matcher looks up those fingerprints and returns candidate backends that recently served matching prefixes.

The cache-aware scorer combines:

- prefix match length and decayed warmth,
- backend health,
- current in-flight pressure,
- latency EWMA,
- error EWMA.

After the request completes, the selected backend is recorded for the request's prefix fingerprints. This is metadata only; the router does not copy or share KV cache. vLLM still owns its own per-process cache.

## Metrics

The router exposes Prometheus metrics on `/metrics`.

Important metrics:

- `router_backend_selected_total{backend}`: how often each backend is selected.
- `router_backend_inflight_requests{backend}`: current in-flight request pressure.
- `router_backend_latency_ewma_ms{backend}`: latency EWMA used by routing.
- `router_backend_error_ewma{backend}`: error EWMA used by routing.
- `router_backend_healthy{backend}`: current backend health state.
- `router_prefix_candidate_matches_total{match_length}`: observed prefix match candidates.
- `router_prefix_match_warmth_bucket`: distribution of decayed prefix warmth.
- `router_prefix_match_age_seconds_bucket`: distribution of prefix metadata age.
- `router_prefix_metadata_evictions_total`: expired prefix metadata.

## Benchmarks

Benchmark details and raw outputs live under:

- `benchmarks/baseline`: direct Kubernetes Service to vLLM, no router.
- `benchmarks/routed`: traffic routed through `k8s-vllm-router`.

The key result is the three-pod shared-prefix workload. This is the case where ordinary Kubernetes load balancing has enough pods for parallelism, but does not know which pod has the warm KV cache.

| Scenario | TTFT p50 | Latency p50 |
| --- | ---: | ---: |
| No router, 3 vLLM pods, shared prefixes | 3161 ms | 9459 ms |
| Router, 3 vLLM pods, shared prefixes | 315 ms | 4097 ms |

No router, three vLLM pods, shared prefixes:

![No router, 3 pods with shared prefixes](https://coosisv.cc/k8s_vllm_router/20260426/3pods_shared.png)

Router, three vLLM pods, shared prefixes:

![Router, 3 pods with shared prefixes](https://coosisv.cc/k8s_vllm_router/20260426/3pods_shared_routed.png)

In this run, routing shared-prefix traffic through the router improved TTFT p50 by about 90% and latency p50 by about 57% compared with a plain three-pod Kubernetes Service.

For context, the no-router three-pod random-prefix run had TTFT p50 of 3301 ms and latency p50 of 9462 ms. That is almost the same p50 latency shape as the no-router shared-prefix run, which is the core problem: the Service gets multi-pod parallelism, but it does not preserve vLLM prefix-cache locality.

These are local Kubernetes/vLLM benchmark numbers, not production GPU serving claims. The important signal is the relative behavior: with shared prefixes, cache-aware routing makes multi-pod vLLM behave much more like the warm-cache case instead of the random-load-balanced case.

## Repository Layout

- `cmd/router`: router entrypoint.
- `internal/router`: routing, matching, health, pressure, and forwarding logic.
- `internal/discovery`: Kubernetes EndpointSlice discovery.
- `internal/metrics`: Prometheus metric recorder.
- `deploy/kubernetes`: example Kubernetes manifests.
- `deploy/docker`: container build files.
- `benchmarks`: baseline and routed benchmark runs.
