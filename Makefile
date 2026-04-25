CAIRO_CFLAGS := $(shell pkg-config --cflags cairo 2>/dev/null)
CAIRO_LDFLAGS := $(shell pkg-config --libs cairo 2>/dev/null)
CGO_FLAGS := CGO_CFLAGS="$(CAIRO_CFLAGS)" CGO_LDFLAGS="$(CAIRO_LDFLAGS)"

.PHONY: all designer demo clean test install-deps example-calculator example-todo gen-resources run

all: gen-resources designer demo

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
		echo "Installing Cairo via apt..."; \
		sudo apt-get install -y libcairo2-dev pkg-config; \
	fi
	go mod tidy
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
	@echo "  make all                Build everything (designer + demo + resources)"
	@echo "  make designer           Build the visual designer"
	@echo "  make demo               Build the widget demo"
	@echo "  make gen-resources      Generate icon/theme PNG resources"
	@echo "  make example-calculator Build the calculator example"
	@echo "  make example-todo       Build the todo app example"
	@echo ""
	@echo "Other:"
	@echo "  make test               Run tests"
	@echo "  make clean              Remove build artifacts"
	@echo "  make help               Show this help"
