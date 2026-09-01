VERSION ?= 0.4.0
DIST := dist
LDFLAGS := -s -w -X ekkoplayer/internal/buildinfo.Version=$(VERSION)

.PHONY: dev dev-backend dev-player dev-admin build build-backend build-player build-admin release-amd64 release-arm64 release-all test clean
dev:
	@echo "Run make dev-backend, make dev-player, and make dev-admin in separate terminals."
dev-backend:
	cd backend && go run ./cmd/playerd
dev-player:
	cd player-ui && npm run dev
dev-admin:
	cd admin-ui && npm run dev
build: build-backend build-player build-admin
build-backend:
	cd backend && CGO_ENABLED=0 go build -ldflags="-X ekkoplayer/internal/buildinfo.Version=$(VERSION)" -o ekkoplayer ./cmd/playerd
build-player:
	cd player-ui && npm run build
build-admin:
	cd admin-ui && npm run build

define release_arch
	test -n '$(VERSION)'
	rm -rf $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)
	mkdir -p $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/player $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/admin
	cd backend && GOOS=linux GOARCH=$(1) CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o ../$(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/ekkoplayer ./cmd/playerd
	cp -a player-ui/dist/. $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/player/
	cp -a admin-ui/dist/. $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/admin/
	cp -a initialize.sh config/install.example.json INSTALL.md AUDIO.md $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/
	cp -a deploy $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/
	printf '%s\n' '$(VERSION)' > $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/VERSION
	printf '%s\n' '$(1)' > $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/ARCH
	chmod 0755 $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/initialize.sh $(DIST)/stage/ekkoplayer_$(VERSION)_linux_$(1)/ekkoplayer
	tar -C $(DIST)/stage -czf $(DIST)/ekkoplayer_$(VERSION)_linux_$(1).tar.gz ekkoplayer_$(VERSION)_linux_$(1)
	cd $(DIST) && sha256sum ekkoplayer_$(VERSION)_linux_$(1).tar.gz > ekkoplayer_$(VERSION)_linux_$(1).tar.gz.sha256
endef

release-amd64: build-player build-admin
	$(call release_arch,amd64)
release-arm64: build-player build-admin
	$(call release_arch,arm64)
release-all: release-amd64 release-arm64
test:
	cd backend && go test -race ./...
	cd player-ui && npm run build
	cd admin-ui && npm run build
clean:
	rm -rf player-ui/dist admin-ui/dist backend/ekkoplayer dist/stage
