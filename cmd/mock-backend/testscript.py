import argparse
import asyncio
import collections
import json
import random
import statistics
import time

import httpx

MODEL = "Qwen/Qwen2.5-0.5B-Instruct"
NUM_COMMON_PREFIXES = 32
BODY_LEN = 1600


def make_body(tag: str, n: int) -> str:
    chunk = f"[{tag}] "
    reps = (n // len(chunk)) + 1
    return (chunk * reps)[:n]


COMMON_PREFIXES = [
    (
        f"You are given reference document #{k}.\n"
        f"{make_body(f'DOC{k:02d}', BODY_LEN)}\n"
        "Use it to answer the question.\n"
    )
    for k in range(NUM_COMMON_PREFIXES)
]


def make_shared_prompt(i: int) -> str:
    prefix = COMMON_PREFIXES[i % NUM_COMMON_PREFIXES]
    return prefix + f"\nQuestion variant #{i}: summarize key points."


def make_random_prompt(i: int) -> str:
    unique_doc = (
        f"{i}-th query: you are given a reference document.\n"
        f"{make_body(f'RAND{i:04d}', BODY_LEN)}\n"
        "Use it to answer the question.\n"
    )
    return unique_doc + f"\nQuestion variant #{i}: summarize key points."


def make_prompt(i: int, shared: bool) -> str:
    return make_shared_prompt(i) if shared else make_random_prompt(i)


async def one_request(client: httpx.AsyncClient, url: str, i: int, shared: bool):
    payload = {
        "model": MODEL,
        "messages": [{"role": "user", "content": make_prompt(i, shared)}],
        "max_tokens": 64,
        "temperature": 0.0,
        "stream": True,
    }

    t0 = time.perf_counter()
    t_first = None
    first_body = {}

    async with client.stream("POST", url, json=payload, timeout=120.0) as resp:
        resp.raise_for_status()
        async for line in resp.aiter_lines():
            if not line:
                continue
            if not line.startswith("data: "):
                continue
            data = line[6:]
            if data.strip() == "[DONE]":
                break
            if t_first is None:
                t_first = time.perf_counter()
                first_body = json.loads(data)
    t1 = time.perf_counter()

    mock = first_body.get("mock", {})
    return {
        "ttft_ms": None if t_first is None else (t_first - t0) * 1000,
        "latency_ms": (t1 - t0) * 1000,
        "backend": first_body.get("backend"),
        "cache_hit": mock.get("cache_hit"),
        "match_length": mock.get("match_length"),
        "sleep_ms": mock.get("sleep_ms"),
        "mock_ttft_ms": mock.get("ttft_ms"),
    }


async def run(url: str, n: int, concurrency: int, shared: bool, seed: int):
    random.seed(seed)
    limits = httpx.Limits(max_keepalive_connections=0, max_connections=concurrency)

    async with httpx.AsyncClient(limits=limits) as client:
        sem = asyncio.Semaphore(concurrency)
        results = []
        errors = 0

        async def wrapped(i):
            nonlocal errors
            async with sem:
                try:
                    results.append(await one_request(client, url, i, shared))
                except Exception as e:
                    errors += 1
                    print(f"request {i} failed: {type(e).__name__}: {e}")

        order = list(range(n))
        random.shuffle(order)
        await asyncio.gather(*(wrapped(i) for i in order))

    lats = [r["latency_ms"] for r in results]
    ttfts = [r["ttft_ms"] for r in results if r["ttft_ms"] is not None]
    sleeps = [r["sleep_ms"] for r in results if r["sleep_ms"] is not None]
    mock_ttfts = [r["mock_ttft_ms"] for r in results if r["mock_ttft_ms"] is not None]
    backends = collections.Counter(r["backend"] for r in results)
    cache_hits = sum(1 for r in results if r["cache_hit"])
    match_lengths = collections.Counter(r["match_length"] for r in results)

    def p(xs, q):
        xs = sorted(xs)
        idx = int((len(xs) - 1) * q)
        return xs[idx] if xs else None

    print(
        json.dumps(
            {
                "n": n,
                "concurrency": concurrency,
                "shared": shared,
                "num_common_prefixes": NUM_COMMON_PREFIXES if shared else 0,
                "attempted": n,
                "succeeded": len(results),
                "failed": errors,
                "lat_p50_ms": p(lats, 0.50),
                "lat_p95_ms": p(lats, 0.95),
                "lat_mean_ms": statistics.mean(lats) if lats else None,
                "ttft_p50_ms": p(ttfts, 0.50),
                "ttft_p95_ms": p(ttfts, 0.95),
                "ttft_mean_ms": statistics.mean(ttfts) if ttfts else None,
                "mock_ttft_p50_ms": p(mock_ttfts, 0.50),
                "mock_sleep_p50_ms": p(sleeps, 0.50),
                "mock_sleep_p95_ms": p(sleeps, 0.95),
                "cache_hits": cache_hits,
                "cache_hit_rate": cache_hits / len(results) if results else None,
                "backends": dict(sorted(backends.items())),
                "match_lengths": dict(sorted(match_lengths.items())),
            },
            indent=2,
        )
    )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://localhost:8080/v1/chat/completions")
    parser.add_argument("--n", type=int, default=100)
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--shared", action="store_true")
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()

    asyncio.run(
        run(
            url=args.url,
            n=args.n,
            concurrency=args.concurrency,
            shared=args.shared,
            seed=args.seed,
        )
    )


if __name__ == "__main__":
    main()
