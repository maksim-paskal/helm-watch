export KUBECONFIG=$(HOME)/.kube/kurento-stage
image=paskalmaksim/helm-watch:dev
namespace=test-helm-watch

test:
	go mod tidy
	go test ./...
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run -v

build:
	go run github.com/goreleaser/goreleaser@latest build --clean --skip=validate --snapshot
	mv ./dist/helm-watch_linux_amd64_v1/helm-watch helm-watch
	docker build --pull --push . -t $(image)

clean:
	kubectl delete ns $(namespace) || true

.PHONY: e2e
e2e:
	go run ./cmd/main.go helm upgrade test-helm-watch \
	./e2e/chart \
	--install \
	--namespace $(namespace) \
	--create-namespace
