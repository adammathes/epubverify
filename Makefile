SPEC_DIR ?= $(HOME)/epubcheck-spec
EPUBCHECK_JAR ?= $(HOME)/tools/epubcheck-5.3.0/epubcheck.jar

.PHONY: build test spec-test compare bench clean

build:                       ## Build the binary
	go build -o epubcheck ./cmd/epubcheck/

test:                        ## Run unit tests
	go test ./pkg/...

spec-test:                   ## Run spec compliance tests
	EPUBCHECK_SPEC_DIR=$(SPEC_DIR) go test ./test/ -v

compare: build               ## Run full comparison via spec scripts
	cd $(SPEC_DIR) && ./scripts/compare-implementation.sh $(CURDIR)/epubcheck

bench: build                 ## Benchmark vs reference epubcheck
	@echo "=== epubcheck-go ===" && time ./epubcheck $(SPEC_DIR)/fixtures/epub/valid/minimal-epub3.epub --json /dev/null 2>/dev/null
	@echo "=== reference java ===" && time java -jar $(EPUBCHECK_JAR) $(SPEC_DIR)/fixtures/epub/valid/minimal-epub3.epub --json /dev/null 2>/dev/null

clean:
	rm -f epubcheck

help:                        ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'
