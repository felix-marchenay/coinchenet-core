.PHONY: test

test:
	docker-compose exec go go test ./test

test-coverage:
	docker-compose exec go go test -cover ./test ./src
	docker-compose exec go go tool cover -func=coverage.out