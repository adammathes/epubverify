SPEC_DIR ?= $(dir $(abspath $(lastword $(MAKEFILE_LIST))))../epubverify-spec
EPUBCHECK_JAR ?= $(HOME)/tools/epubcheck-5.3.0/epubcheck.jar

.PHONY: build test spec-test compare realworld-test realworld-compare bench clean help

build:                       ## Build the binary
	go build -o epubverify .

test:                        ## Run unit tests
	go test ./pkg/...

spec-test:                   ## Run spec compliance tests
	EPUBCHECK_SPEC_DIR=$(SPEC_DIR) go test ./test/ -v

compare: build               ## Run full comparison via spec scripts
	cd $(SPEC_DIR) && ./scripts/compare-implementation.sh $(CURDIR)/epubverify

realworld-test: build        ## Run real-world sample tests (download samples first)
	go test ./test/realworld/ -v

realworld-compare: build     ## Compare epubverify vs epubcheck on real-world samples
	EPUBCHECK_JAR=$(EPUBCHECK_JAR) ./test/realworld/compare.sh

bench: build                 ## Benchmark vs reference epubcheck
	@echo "=== epubverify ===" && time ./epubverify $(SPEC_DIR)/fixtures/epub/valid/minimal-epub3.epub --json /dev/null 2>/dev/null
	@echo "=== reference java ===" && time java -jar $(EPUBCHECK_JAR) $(SPEC_DIR)/fixtures/epub/valid/minimal-epub3.epub --json /dev/null 2>/dev/null

clean:
	rm -f epubverify

help:                        ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'
