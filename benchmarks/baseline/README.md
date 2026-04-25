# Benchmarks
This directory contains benchmarks for baseline vllm instances with `Qwen/Qwen2.5-1.5B-Instruct` model. 
Each test directory contains: `deployment.yaml`, `service.yaml` and `runtest.py`. The 
result of each test is stored in `results/`.
Directories that have `shared` in their name means the queries share a long common prefix, 
which is expected to benefit from vllm caching.
Directories that start with `single_` are tests with a single vllm instance,
while those that start with a number (e.g., `3_`) are tests with multiple vllm instances.
