CAIRO_CFLAGS := $(shell pkg-config --cflags cairo 2>/dev/null)
CAIRO_LDFLAGS := $(shell pkg-config --libs cairo 2>/dev/null)
CGO_FLAGS := CGO_CFLAGS="$(CAIRO_CFLAGS)" CGO_LDFLAGS="$(CAIRO_LDFLAGS)"
ifeq ($(OS),Windows_NT)
IDE_EXE_SUFFIX := .exe
endif

.PHONY: all designer demo clean test install-deps example-calculator example-todo gen-resources run ide run-ide test-ide silkgen test-platform-scripts

all: gen-resources ide silkgen designer demo

ide: ide-tools
	@mkdir -p bin
	$(CGO_FLAGS) go build -ldflags="-X github.com/uk0/silk/internal/idetools.RequireBundle=1" -o bin/silkide$(IDE_EXE_SUFFIX) ./cmd/silkide

.PHONY: ide-tools
ide-tools:
	go run ./cmd/silktools -out bin/tools

# Native Windows builds use MSYS2's existing resource compiler. The generated
# architecture-specific .syso is ignored on other operating systems.
ifeq ($(OS),Windows_NT)
ide: cmd/silkide/rsrc_windows_amd64.syso
cmd/silkide/rsrc_windows_amd64.syso: assets/branding/silkide.ico assets/branding/silkide.rc scripts/build-windows-icon.sh
	bash scripts/build-windows-icon.sh
endif

.PHONY: bundle-ide
bundle-ide: ide
	@mkdir -p bin/icon
	@cp -R icon/. bin/icon/
	bash scripts/package-silkide-branding.sh "$$(go env GOOS)" bin
	go run ./cmd/silktools -verify -out bin/tools
	@if [ "$$(go env GOOS)" = "darwin" ]; then \
		go run ./cmd/silktools -verify -out bin/SilkIDE.app/Contents/MacOS/tools; \
	fi

silkgen:
	@mkdir -p bin
	$(CGO_FLAGS) go build -o bin/silkgen ./cmd/silkgen

run-ide: ide
	./bin/silkide$(IDE_EXE_SUFFIX)

.PHONY: test-ide-lsp
test-ide-lsp: ide-tools
	bash scripts/test-bundled-gopls.sh bin

test-ide: test-ide-lsp test-sdk-utilities
	$(CGO_FLAGS) go test ./internal/idetools ./cmd/silktools ./assets/branding ./printing ./pdfexport ./cmd/silkide ./cmd/silkgen ./internal/silkgen ./decl ./gui ./ged ./prop ./graph ./paint ./locator ./filesearch ./snippet ./buildissues

.PHONY: test-sdk-utilities
test-sdk-utilities:
	$(CGO_FLAGS) go test -race -short -count=1 ./async ./httpx ./jsonx ./internal/filetxn ./examples/download-monitor ./examples/json-inspector

test-platform-scripts:
	bash scripts/test-platform-scripts.sh

designer:
	@mkdir -p bin
	$(CGO_FLAGS) go build -o bin/silk-designer ./design.go

demo:
	@mkdir -p bin
	$(CGO_FLAGS) go build -o bin/silk-demo ./demo.go

example-calculator:
	@mkdir -p bin
	$(CGO_FLAGS) go build -o bin/silk-calculator ./examples/calculator/main.go

example-todo:
	@mkdir -p bin
	$(CGO_FLAGS) go build -o bin/silk-todo ./examples/todoapp/main.go

example-quickstart:
	@mkdir -p bin
	$(CGO_FLAGS) go build -o bin/silk-login ./examples/quickstart/main.go

test:
	$(CGO_FLAGS) go test ./core/... ./geom/... ./paint/... ./graph/...

clean:
	rm -rf bin/

install-deps:
	@echo "=== Installing dependencies ==="
	@if [ "$$(uname)" = "Darwin" ]; then \
		echo "Installing Cairo via Homebrew..."; \
		brew install cairo pkg-config; \
	fi
	@if [ "$$(uname)" = "Linux" ]; then \
		bash scripts/install-linux-deps.sh; \
	fi
	go mod download
	@echo "=== Done ==="

gen-resources:
	@echo "Generating icon and theme resources..."
	@go run tools/gen_resources.go bin
	@echo "Resources generated in bin/"

run: designer gen-resources
	@echo "Starting Silk Designer..."
	@cd bin && ./silk-designer

help:
	@echo "Silk - Go Cross-Platform UI Framework & Visual Designer"
	@echo ""
	@echo "Quick start:"
	@echo "  make install-deps       Install Cairo and dependencies"
	@echo "  make run                Build and run the designer"
	@echo ""
	@echo "Build targets:"
	@echo "  make all                Build IDE, generator, designer, demo and resources"
	@echo "  make designer           Build the visual designer"
	@echo "  make demo               Build the widget demo"
	@echo "  make ide                Build the integrated Go designer / IDE"
	@echo "  make bundle-ide         Bundle IDE, pinned gopls and native launcher assets"
	@echo "  make ide-tools          Build/repair the pinned bundled gopls"
	@echo "  make silkgen            Build the Go form generator"
	@echo "  make run-ide            Build and run the IDE"
	@echo "  make test-ide           Run IDE regressions including required bundled LSP"
	@echo "  make test-ide-lsp       Build tools and require real bundled LSP integration"
	@echo "  make test-sdk-utilities Test async, HTTP/JSON, file transactions and examples"
	@echo "  make test-platform-scripts Check dependency and release-packaging scripts"
	@echo "  make gen-resources      Generate icon/theme PNG resources"
	@echo "  make example-calculator Build the calculator example"
	@echo "  make example-todo       Build the todo app example"
	@echo ""
	@echo "Other:"
	@echo "  make test               Run tests"
	@echo "  make clean              Remove build artifacts"
	@echo "  make help               Show this help"
