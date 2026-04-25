import asyncio
import json
import random
import statistics
import time

import httpx

URL = "http://localhost:30976/v1/chat/completions"
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


async def one_request(client: httpx.AsyncClient, i: int, shared: bool):
    payload = {
        "model": MODEL,
        "messages": [{"role": "user", "content": make_prompt(i, shared)}],
        "max_tokens": 64,
        "temperature": 0.0,
        "stream": True,
    }

    t0 = time.perf_counter()
    t_first = None

    async with client.stream("POST", URL, json=payload, timeout=120.0) as resp:
        resp.raise_for_status()
        async for line in resp.aiter_lines():
            if not line:
                continue
            if line.startswith("data: ") and t_first is None:
                data = line[6:]
                if data.strip() != "[DONE]":
                    t_first = time.perf_counter()

    t1 = time.perf_counter()

    return {
        "ttft_ms": None if t_first is None else (t_first - t0) * 1000,
        "latency_ms": (t1 - t0) * 1000,
    }


async def run(n=100, concurrency=8, shared=True, seed=42):
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
                    r = await one_request(client, i, shared)
                    results.append(r)
                except Exception as e:
                    errors += 1
                    print(f"request {i} failed: {type(e).__name__}: {e}")

        order = list(range(n))
        random.shuffle(order)
        await asyncio.gather(*(wrapped(i) for i in order))

    ttfts = [r["ttft_ms"] for r in results if r["ttft_ms"] is not None]
    lats = [r["latency_ms"] for r in results]

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
                "ttft_p50_ms": p(ttfts, 0.50),
                "ttft_p95_ms": p(ttfts, 0.95),
                "lat_p50_ms": p(lats, 0.50),
                "lat_p95_ms": p(lats, 0.95),
                "ttft_mean_ms": statistics.mean(ttfts) if ttfts else None,
                "lat_mean_ms": statistics.mean(lats) if lats else None,
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    asyncio.run(run(n=100, concurrency=8, shared=True))
