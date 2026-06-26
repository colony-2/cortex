.PHONY: build test fmt server-build web-build cortex clean

build: cortex

server-build:
	go build ./server/cmd/api ./server/cmd/uitestserver ./server/cmd/cortex

web-build:
	pnpm --filter @colony2/app build

cortex: web-build
	mkdir -p server/internal/webdist/dist
	find server/internal/webdist/dist -mindepth 1 ! -name placeholder.txt -exec rm -rf {} +
	cp -R web/app/dist/. server/internal/webdist/dist/
	go build -o build/cortex ./server/cmd/cortex

test:
	go test ./server/...
	pnpm --filter @colony2/app test

fmt:
	gofmt -w server
	pnpm --filter @colony2/app typecheck

clean:
	rm -rf build
	find server/internal/webdist/dist -mindepth 1 ! -name placeholder.txt -exec rm -rf {} +
