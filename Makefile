FUZZTIME ?= 30s
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all web build test fuzz lint demo clean kev

all: build

web:
	cd web && npm ci && npm run check && npm run build
	rm -rf cmd/netzwart/web && mkdir -p cmd/netzwart/web && cp -r web/dist/. cmd/netzwart/web/ && touch cmd/netzwart/web/.gitkeep

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/netzwart ./cmd/netzwart

test:
	go vet ./...
	go test -race ./...

fuzz:
	@for pkg in ./internal/proto ./internal/textutil ./internal/l2 ./internal/fritzbox ./internal/software; do \
	  for f in $$(go test $$pkg -list '^Fuzz' | grep ^Fuzz); do \
	    go test $$pkg -run '^$$' -fuzz "^$$f$$" -fuzztime $(FUZZTIME) || exit 1; \
	  done; \
	done

# Refresh the snapshot of known exploited vulnerabilities (docs/09, F6)
# from the current state of both sources; run before every release.
kev:
	go run ./tools/kevgen \
	  -kev $$(git ls-remote https://github.com/cisagov/kev-data HEAD | cut -f1) \
	  -cve $$(git ls-remote https://github.com/CVEProject/cvelistV5 HEAD | cut -f1) \
	  -out internal/vuln/kev.json

demo: build
	mkdir -p data && NETZWART_DEMO=1 NETZWART_DATA=$(CURDIR)/data ./bin/netzwart

clean:
	rm -rf bin data web/dist
	find cmd/netzwart/web -mindepth 1 ! -name .gitkeep -delete
