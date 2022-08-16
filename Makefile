get:
	cd v2 && go get -v -t -d ./...

test:
	cd v2 && go test -v -race ./...

test_cover:
	cd v2 && ../test_cover.sh

.PHONY: get test test_cover
