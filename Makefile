build:
	go build

debug:
	dlv debug

test:
	go test -tags=test -- -T
