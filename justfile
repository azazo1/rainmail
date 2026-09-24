# 构建信息, 注入 internal/version
commit := `git rev-parse --short HEAD 2>/dev/null || echo unknown`
build_date := `date -u +%Y-%m-%dT%H:%M:%SZ`
ldflags := "-s -w -X github.com/azazo1/rainmail/internal/version.Commit=" + commit + " -X github.com/azazo1/rainmail/internal/version.Date=" + build_date

[private]
default:
    @just --list

# 构建当前平台的二进制到 dist/rainmail
build:
    go build -trimpath -ldflags "{{ldflags}}" -o dist/rainmail .

# 运行单元测试
test:
    go test ./...

# 静态检查
lint:
    go vet ./...

# 生成带注释的示例配置
init-config *args:
    go run . config init {{args}}

# 立刻检查一次, 不发送提醒
check *args:
    go run . check --dry-run {{args}}

# 常驻运行, 前台轮询
run *args:
    go run . run {{args}}

# 发送测试邮件
test-email *args:
    go run . test email {{args}}

# 弹出测试通知
test-notify *args:
    go run . test system {{args}}

# 交叉编译三端产物到 dist/
build-all:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
        os="${target%/*}"
        arch="${target#*/}"
        ext=""
        if [ "$os" = "windows" ]; then ext=".exe"; fi
        echo "building ${os}/${arch}"
        GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
            go build -trimpath -ldflags "{{ldflags}}" -o "dist/rainmail-${os}-${arch}${ext}" .
    done
