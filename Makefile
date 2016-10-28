get:
	go get -v -t -d ./...

test:
	go test -v -race ./...

test_cover:
	./test_cover.sh

.PHONY: get test
