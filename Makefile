get:
	go get -d ./...

test:
	go test -v ./...

.PHONY: get test
