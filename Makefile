.PHONY: test

test:
	docker-compose exec go go test ./...

test-coverage:
	docker-compose exec go go test -coverprofile=coverage.out ./...
	docker-compose exec go go tool cover -func=coverage.out