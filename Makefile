.PHONY: all clean editor todo

all: editor
	go vet
	go install
	make todo

editor:
	go fmt
	go test -i
	go test
	go build

todo:
	@grep -n ^[[:space:]]*_[[:space:]]*=[[:space:]][[:alpha:]][[:alnum:]]* *.go || true
	@grep -n TODO *.go || true
	@grep -n BUG *.go || true
	@grep -n println *.go || true

clean:
	@go clean
	rm -f y.output
