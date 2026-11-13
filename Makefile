.PHONY: all build check clean doc help install install-dev lint test
.DEFAULT_GOAL := help

# Define a helper for checking command existence
check-command = @if ! command -v $(1) > /dev/null; then \
		echo "Error: '$(1)' not found. $(2)"; \
		exit 1; \
	fi

help:
	@echo "Available targets:"
	@echo ""
	@echo "Development:"
	@echo "  all          - Build, test, and check (wraps scripts/all.bash)"
	@echo "  build        - Build minimega binaries (wraps scripts/build.bash)"
	@echo "  check        - Run gofmt and go vet (wraps scripts/check.bash)"
	@echo "  doc          - Build documentation (wraps scripts/doc.bash)"
	@echo "  lint         - Run all prek hooks across the whole repository"
	@echo "  test         - Run unit tests (wraps scripts/test.bash)"
	@echo ""
	@echo "Installation:"
	@echo "  install      - Install minimega to the system (wraps scripts/install.bash)"
	@echo "  install-dev  - Install local dev tooling (prek) and register git hooks"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean        - Remove build artifacts (wraps scripts/clean.bash)"
	@echo ""
	@echo "Help:"
	@echo "  help         - Show this help message"

all:
	./scripts/all.bash

build:
	./scripts/build.bash

check:
	./scripts/check.bash

doc:
	./scripts/doc.bash

test:
	./scripts/test.bash

install:
	./scripts/install.bash

clean:
	./scripts/clean.bash

lint:
	@command -v prek > /dev/null || { echo "Error: 'prek' not found. Run 'make install-dev' first."; exit 1; }
	prek run --all-files

install-dev:
	@command -v prek > /dev/null || pip install 'prek>=0.5.10'
	prek install
