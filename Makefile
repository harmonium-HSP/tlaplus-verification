.PHONY: verify verify-redlock verify-raft verify-paxos test chaos clean build generate-from-tla deploy-redlock-operator deploy-raft-operator create-redlock-cluster create-raft-cluster status test-paxos chaos-paxos test-leaselock chaos-leaselock

verify: verify-redlock verify-raft verify-paxos

verify-all: verify

verify-redlock:
	@cd models/redlock && java -cp ../../tla2tools.jar tlc.TLC -config redlock_optimized.cfg redlock_optimized.tla

verify-raft:
	@scripts/verify-raft.sh

verify-paxos:
	@scripts/verify-paxos.sh

test:
	go test -v ./pkg/...

test-all: test

test-paxos:
	go test -v ./pkg/paxos/...

test-leaselock:
	go test -v ./pkg/leaselock/...

chaos:
	go test -v -run TestChaos -count=10 ./pkg/...

chaos-paxos:
	go test -v -run TestChaos -count=5 ./pkg/paxos/...

chaos-leaselock:
	go test -v -run TestChaos -count=5 ./pkg/leaselock/...

clean:
	rm -f models/**/*.out models/**/*.log
	rm -f bin/*

build:
	go build -o bin/redlock-server ./cmd/redlock-server
	go build -o bin/tla-gen ./cmd/tla-gen

build-redlock-operator:
	cd operators/redlock-operator && go build -o ../../bin/redlock-operator ./cmd/main.go

build-raft-operator:
	cd operators/raft-operator && go build -o ../../bin/raft-operator ./cmd/main.go

generate-from-tla:
	go run cmd/tla-gen/main.go -model models/redlock/redlock_optimized.tla -out pkg/redlock/

benchmark:
	go test -bench=. -benchmem ./pkg/redlock/...

race:
	go test -race -v ./pkg/...

docker-up:
	docker-compose -f deployments/docker-compose.yml up -d

docker-down:
	docker-compose -f deployments/docker-compose.yml down

docker-build-redlock-operator:
	docker build -t redlock-operator:latest -f operators/redlock-operator/Dockerfile operators/redlock-operator

docker-build-raft-operator:
	docker build -t raft-operator:latest -f operators/raft-operator/Dockerfile operators/raft-operator

deploy-redlock-operator: docker-build-redlock-operator
	kubectl apply -f deployments/kubernetes/redlock-operator.yaml

deploy-raft-operator: docker-build-raft-operator
	kubectl apply -f deployments/kubernetes/raft-operator.yaml

create-redlock-cluster:
	kubectl apply -f examples/redlock-cluster.yaml

create-raft-cluster:
	kubectl apply -f examples/raft-cluster.yaml

status:
	@echo "=== Redlock Clusters ==="
	@kubectl get redlockclusters 2>/dev/null || echo "No Redlock clusters found"
	@echo ""
	@echo "=== Raft Clusters ==="
	@kubectl get raftclusters 2>/dev/null || echo "No Raft clusters found"
	@echo ""
	@echo "=== Operators ==="
	@kubectl get deployments -n redlock-operator 2>/dev/null || echo "Redlock operator not deployed"
	@kubectl get deployments -n raft-operator 2>/dev/null || echo "Raft operator not deployed"

undeploy-redlock-operator:
	kubectl delete -f deployments/kubernetes/redlock-operator.yaml

undeploy-raft-operator:
	kubectl delete -f deployments/kubernetes/raft-operator.yaml

delete-redlock-cluster:
	kubectl delete -f examples/redlock-cluster.yaml

delete-raft-cluster:
	kubectl delete -f examples/raft-cluster.yaml

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy
