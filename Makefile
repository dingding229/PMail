VERSION ?= 0.0.5
GITHASH ?= unknown
export VERSION

build: build_web build_server

clean:
	rm -rf output


build_web:
	cd web && npm ci --ignore-scripts && npm run build

check-version:
	@case "$$VERSION" in *[!0-9.]*|'') echo 'PMail Pro version must match ^0\.[0-9]+\.[0-9]+$$' >&2; exit 1;; esac; \
	printf '%s\n' "$$VERSION" | grep -Eq '^0\.[0-9]+\.[0-9]+$$' || { echo 'PMail Pro version must match ^0\.[0-9]+\.[0-9]+$$' >&2; exit 1; }

build_server: check-version
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.goVersion=$(go version)' -X 'main.gitHash=$(GITHASH)' -X 'main.buildTime=$(TZ=UTC-8 date +%Y-%m-%d" "%H:%M:%S)'" -o pmail_linux_amd64  main.go
	cd server && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.goVersion=$(go version)' -X 'main.gitHash=$(GITHASH)' -X 'main.buildTime=$(TZ=UTC-8 date +%Y-%m-%d" "%H:%M:%S)'" -o pmail_windows_amd64.exe  main.go
	cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.goVersion=$(go version)' -X 'main.gitHash=$(GITHASH)' -X 'main.buildTime=$(TZ=UTC-8 date +%Y-%m-%d" "%H:%M:%S)'" -o pmail_mac_amd64  main.go
	cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.goVersion=$(go version)' -X 'main.gitHash=$(GITHASH)' -X 'main.buildTime=$(TZ=UTC-8 date +%Y-%m-%d" "%H:%M:%S)'" -o pmail_mac_arm64  main.go

test:
	cd server && go test -v -p 1 ./...

test_mysql:
	cd server && go test -tags integration -v -p 1 . -args mysql

test_postgres:
	export PMAIL_TEST_PG_DSN="$${PMAIL_TEST_PG_DSN:-postgres://postgres:githubTest@postgres:5432/pmail?sslmode=disable}" && cd server && go test -tags integration -v -p 1 ./... -args postgres

.PHONY: build clean check-version build_web build_server docker test test_mysql test_postgres
docker: check-version
	docker build --build-arg VERSION="$$VERSION" -t pmail:local .
