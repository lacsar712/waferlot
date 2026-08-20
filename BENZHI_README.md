# Waferlot

鏅跺渾 lot-step MES 杞彂

## Build

```bash
export GOTOOLCHAIN=local
go build ./...
```

## Test

```bash
export GOTOOLCHAIN=local
go test ./... -count=1
```

## Docker (benzhi)

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh waferlot linux/amd64
./build_benzhi_docker.sh waferlot linux/arm64
```