.PHONY: run build tidy fmt test docker-build docker-up

run:
	go run ./services/api/cmd/server

build:
	go build -o bin/api ./services/api/cmd/server

tidy:
	go mod tidy

fmt:
	go fmt ./...

test:
	go test ./...

docker-build:
	docker build -f services/api/Dockerfile -t recall-api .

docker-up:
	docker compose up --build

docker-down:
	docker compose down
