.PHONY: run build vet tidy docker-build deploy deploy-local vercel-deploy

# go build appends .exe automatically on Windows; the deployed artifact is
# always a Linux binary, built explicitly in the deploy target.
BINARY := bin/api

run:
	go run ./cmd/api

build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api

vet:
	go vet ./...

tidy:
	go mod tidy

docker-build:
	docker build -t truemoney-voucher -f deployments/Dockerfile .

deploy: build
	@echo "Building Linux binary..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY)-linux ./cmd/api
	@echo "Deploying to remote server..."
	ssh zelthr@192.168.1.111 "mkdir -p /home/zelthr/truemoney-voucher"
	scp $(BINARY)-linux zelthr@192.168.1.111:/home/zelthr/truemoney-voucher/api
	scp .env.example zelthr@192.168.1.111:/home/zelthr/truemoney-voucher/
	ssh zelthr@192.168.1.111 "cd /home/zelthr/truemoney-voucher && chmod +x api && ./api &"
	@echo "Deployment complete! Service running on http://192.168.1.111:3000"

deploy-local:
	@echo "Deploying locally with Docker..."
	docker build -t truemoney-voucher -f deployments/Dockerfile .
	docker run -d -p 3000:3000 --name truemoney-voucher truemoney-voucher
	@echo "Local deployment complete! Service running on http://localhost:3000"

vercel-deploy:
	vercel --prod