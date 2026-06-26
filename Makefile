.PHONY: build test fmt server-build web-build cortex clean

build: cortex

server-build:
	go build ./server/cmd/api ./server/cmd/uitestserver ./server/cmd/cortex

web-build:
	pnpm --filter @colony2/app build

cortex: web-build
	rm -rf server/internal/webdist/dist
	mkdir -p server/internal/webdist/dist
	cp -R web/app/dist/. server/internal/webdist/dist/
	go build -o build/cortex ./server/cmd/cortex

test:
	go test ./server/...
	pnpm --filter @colony2/app test

fmt:
	gofmt -w server
	pnpm --filter @colony2/app typecheck

clean:
	rm -rf build server/internal/webdist/dist
