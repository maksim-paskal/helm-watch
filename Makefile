KUBECONFIG=$(HOME)/.kube/kurento-stage
image=paskalmaksim/helm-watch:dev

run:
	rm -rf ./logs
	mkdir logs
	go run ./cmd internal wait-for-jobs --namespace=eee -filter test=value

test:
	go mod tidy
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run -v

build:
	docker build --pull --load . -t $(image)

publish:
	docker build --platform=linux/amd64,linux/arm64 --pull --push . -t $(image)

restart:
	kubectl -n paket-images-preloader rollout restart ds paket-images-preloader

install:
	go build -o /tmp/helm-watch ./cmd