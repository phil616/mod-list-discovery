.PHONY: test build clean server client package
export CGO_ENABLED=0
VERSION ?= $(shell cat VERSION)
test:
	python3 -m unittest discover -s tools -p 'test_*.py'
	cd server && test -z "$$(gofmt -l .)" && go vet ./... && go test ./...
	$(MAKE) server
	cd client && MLD_SERVER_BINARY="$(CURDIR)/dist/mod-list-discovery-server" ./gradlew --no-daemon test -PmodVersion="$(VERSION)"
build: server client
server:
	python3 tools/release.py validate "$(VERSION)"
	mkdir -p dist
	cd server && GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X mod-list-discovery/server/internal/app.Version=$(VERSION)" -o ../dist/mod-list-discovery-server ./cmd/mod-list-discovery-server
client:
	python3 tools/release.py validate "$(VERSION)"
	cd client && ./gradlew --no-daemon build -PmodVersion="$(VERSION)"
	mkdir -p dist
	cp "client/build/libs/mod-list-discovery-$(VERSION)+mc26.2.jar" dist/
package: build
	python3 tools/release.py package "$(VERSION)"
clean:
	rm -rf dist server/coverage.out client/build
