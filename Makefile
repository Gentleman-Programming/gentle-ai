.PHONY: test vet lint

test:
	go test ./... -count=1

vet:
	go vet ./...

lint:
	# placeholder for golangci-lint if installed
	@echo "Run golangci-lint manually if available"

