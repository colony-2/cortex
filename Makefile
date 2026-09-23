.PHONY: build test test-e2e typecheck fmt server-build web-build web-embed cortex clean

GO := bash scripts/go.sh

build: cortex

server-build:
	$(GO) build ./server/cmd/api ./server/cmd/uitestserver ./server/cmd/cortex

web-build:
	pnpm --filter @colony2/shared build
	pnpm --filter @colony2/app build

web-embed: web-build
	mkdir -p server/internal/webdist/dist
	find server/internal/webdist/dist -mindepth 1 ! -name placeholder.txt -exec rm -rf {} +
	cp -R web/app/dist/. server/internal/webdist/dist/

cortex: web-embed
	$(GO) build -o build/cortex ./server/cmd/cortex

test:
	$(GO) test ./server/...
	pnpm --filter @colony2/shared test
	pnpm --filter @colony2/app test

fmt:
	gofmt -w server

typecheck:
	pnpm --filter @colony2/shared typecheck
	pnpm --filter @colony2/app typecheck

test-e2e:
	pnpm test:e2e

clean:
	rm -rf build
	find server/internal/webdist/dist -mindepth 1 ! -name placeholder.txt -exec rm -rf {} +
