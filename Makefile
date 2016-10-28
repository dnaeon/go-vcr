get:
	go get -v -t -d ./...

test:
	./test.sh

.PHONY: get test
