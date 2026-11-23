VERSION ?= 0.0.2
GITHASH ?= unknown
export VERSION

build: package

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

package: build_web build_server clean
	mkdir output
	mv server/pmail* output/
	mkdir output/config
	cp -r server/config/dkim output/config/
	cp -r server/config/ssl output/config/
	cp -r server/config/config.json output/config/
	cp -R web/dist output/web
	cp README.md output/

test:
	cd server && go test -v -p 1 ./...

test_mysql:
	cd server && go test -tags integration -v -p 1 . -args mysql

test_postgres:
	export PMAIL_TEST_PG_DSN="$${PMAIL_TEST_PG_DSN:-postgres://postgres:githubTest@postgres:5432/pmail?sslmode=disable}" && cd server && go test -tags integration -v -p 1 ./... -args postgres

.PHONY: build clean check-version build_web build_server package docker compose-init compose-up compose-upgrade compose-backup compose-logs compose-status compose-down
docker: check-version
	docker build --build-arg VERSION="$$VERSION" -t pmail:local .

compose-init:
	./deploy/deploy.sh init

compose-up:
	./deploy/deploy.sh up

compose-upgrade:
	./deploy/deploy.sh upgrade

compose-backup:
	./deploy/deploy.sh backup

compose-logs:
	./deploy/deploy.sh logs

compose-status:
	./deploy/deploy.sh status

compose-down:
	./deploy/deploy.sh down

.PHONY: compose-restore
compose-restore:
	./deploy/deploy.sh restore "$(FILE)"
