# Makefile for GiTK — GTK4 Git GUI Client
#
# Targets:
#   make build       — Compile the binary
#   make run         — Build and run
#   make test        — Run all tests
#   make vet         — Run go vet
#   make lint        — Run vet + staticcheck (if installed)
#   make install     — Install binary + desktop file + icon to ~/.local
#   make uninstall   — Remove installed files
#   make flatpak-build — Build the Flatpak package
#   make clean       — Remove build artifacts

# Application metadata
APP_ID    := io.github.MedaiP90.GiTK
APP_NAME  := gitk
VERSION   := 0.1.0

# Go build settings
GO        := go
GOFLAGS   :=
LDFLAGS   := -s -w -X main.version=$(VERSION)
BUILD_DIR := build
BINARY    := $(BUILD_DIR)/$(APP_NAME)

# Installation directories (freedesktop.org standard)
PREFIX       := $(HOME)/.local
BINDIR       := $(PREFIX)/bin
DATADIR      := $(PREFIX)/share
DESKTOPDIR   := $(DATADIR)/applications
ICONDIR      := $(DATADIR)/icons/hicolor/scalable/apps
METAINFODIR  := $(DATADIR)/metainfo

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

# Build and run
.PHONY: run
run: build
	@echo "Running $(APP_NAME)..."
	$(BINARY)

# Run all tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GO) test ./... -v

# Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

# Run all linters
.PHONY: lint
lint: vet
	@echo "Running staticcheck (if installed)..."
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

# Install to ~/.local following freedesktop.org standards
.PHONY: install
install: build
	@echo "Installing $(APP_NAME) to $(PREFIX)..."
	install -Dm755 $(BINARY) $(BINDIR)/$(APP_NAME)
	install -Dm644 data/$(APP_ID).desktop $(DESKTOPDIR)/$(APP_ID).desktop
	install -Dm644 data/$(APP_ID).metainfo.xml $(METAINFODIR)/$(APP_ID).metainfo.xml
	install -Dm644 data/icons/hicolor/scalable/apps/$(APP_ID).svg $(ICONDIR)/$(APP_ID).svg
	@echo "Installed. You may need to run: update-desktop-database $(DESKTOPDIR)"

# Remove installed files
.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(APP_NAME)..."
	rm -f $(BINDIR)/$(APP_NAME)
	rm -f $(DESKTOPDIR)/$(APP_ID).desktop
	rm -f $(METAINFODIR)/$(APP_ID).metainfo.xml
	rm -f $(ICONDIR)/$(APP_ID).svg

# Build Flatpak package
.PHONY: flatpak-build
flatpak-build:
	@echo "Building Flatpak..."
	flatpak-builder --force-clean $(BUILD_DIR)/flatpak $(APP_ID).flatpak-manifest.yml

# Remove build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)

# Show help
.PHONY: help
help:
	@echo "GiTK Makefile targets:"
	@echo "  build         - Compile the binary"
	@echo "  run           - Build and run"
	@echo "  test          - Run all tests"
	@echo "  vet           - Run go vet"
	@echo "  lint          - Run vet + staticcheck"
	@echo "  install       - Install to ~/.local"
	@echo "  uninstall     - Remove installed files"
	@echo "  flatpak-build - Build Flatpak package"
	@echo "  clean         - Remove build artifacts"
