export KUBECONFIG=$(HOME)/.kube/kurento-stage
image=paskalmaksim/helm-watch:$(shell git rev-parse --short HEAD)
namespace=test-helm-watch

test:
	go mod tidy
	go test ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run -v

build:
	docker build --platform linux/amd64,linux/arm64 -f Dockerfile.local --pull --push . -t $(image)

clean:
	kubectl delete ns $(namespace) || true

.PHONY: e2e
e2e:
	go run ./cmd/main.go helm upgrade test-helm-watch \
	./e2e/chart \
	--install \
	--namespace $(namespace) \
	--create-namespace
