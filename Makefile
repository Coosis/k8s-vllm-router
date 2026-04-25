.PHONY: test run mock-backend run-mock-backends docker-build kind-up deploy benchmark

test:
	go test ./...

run:
	go run ./cmd/router

mock-backend:
	go run ./cmd/mock-backend

run-mock-backends:
	scripts/run-mock-backends.sh

docker-build:
	docker build -f deploy/docker/router.Dockerfile -t k8s-vllm-router:dev .
	docker build -f deploy/docker/mock-backend.Dockerfile -t k8s-vllm-mock-backend:dev .

kind-up:
	kind create cluster --name k8s-vllm-router

deploy:
	kubectl apply -f deploy/kubernetes

benchmark:
	python benchmarks/router/runtest.py
