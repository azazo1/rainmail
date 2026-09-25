[private]
default:
    @just --list

# 构建当前平台的二进制到 dist/rainmail (开发构建, 版本显示 dev-build)
build:
    go build -trimpath -o dist/rainmail .

# 运行单元测试
test:
    go test ./...

# 静态检查
lint:
    go vet ./...

# 检查 Go 代码格式 (只报告, 不自动修改)
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted="$(gofmt -l .)"
    if [[ -n "$unformatted" ]]; then
        echo "以下文件未通过 gofmt, 请手工格式化:"
        echo "$unformatted"
        exit 1
    fi
    echo "gofmt 检查通过"

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

# 交叉编译三端产物到 dist/dev/ (仅供本地试跑, 不注入版本号)
build-all:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist/dev
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
        os="${target%/*}"
        arch="${target#*/}"
        ext=""
        if [ "$os" = "windows" ]; then ext=".exe"; fi
        echo "building ${os}/${arch}"
        GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
            go build -trimpath -o "dist/dev/rainmail-${os}-${arch}${ext}" .
    done

# 根据当前平台生成发布产物 (注入版本号, 输出 dist/rainmail-<版本>-<平台>-<架构>.tar.gz)
[macos]
dist:
    PROJECT_BUILD_VERSION="v$(bash scripts/build-version.sh)" bash scripts/dist.sh

# 根据当前平台生成发布产物 (注入版本号, 输出 dist/rainmail-<版本>-<平台>-<架构>.tar.gz)
[linux]
dist:
    PROJECT_BUILD_VERSION="v$(bash scripts/build-version.sh)" bash scripts/dist.sh

# 根据当前平台生成发布产物 (注入版本号, 输出 dist/rainmail-<版本>-<平台>-<架构>.zip)
[windows]
[script('powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File')]
dist:
    $ErrorActionPreference = 'Stop'
    $env:PROJECT_BUILD_VERSION = "v$(& 'scripts/build-version.ps1' | Out-String).Trim()"
    & 'scripts/dist.ps1'
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
