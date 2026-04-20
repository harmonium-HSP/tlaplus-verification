<#
.SYNOPSIS
一键启动 Redlock + Fencing Token 演示环境

.DESCRIPTION
启动 Docker Compose 服务，等待 Redis 就绪，安装 Go 依赖，验证环境
#>

Write-Host "=== Redlock + Fencing Token Demo Setup ===" -ForegroundColor Cyan

# 切换到项目根目录
$projectRoot = Split-Path $PSScriptRoot -Parent
Set-Location $projectRoot

# 1. 启动 Docker Compose
Write-Host "`n1. Starting Docker containers..." -ForegroundColor Yellow
docker-compose up -d

# 2. 等待 Redis 就绪
Write-Host "`n2. Waiting for Redis instances to be ready..." -ForegroundColor Yellow
$readyCount = 0
$maxAttempts = 30
$attempt = 0

while ($readyCount -lt 5 -and $attempt -lt $maxAttempts) {
    $readyCount = 0
    for ($i = 6379; $i -le 6383; $i++) {
        try {
            $result = docker exec "redlock-fencing-redis-$($i - 6378)" redis-cli ping 2>&1
            if ($result -eq "PONG") {
                $readyCount++
                Write-Host "   ✓ Redis $($i - 6378) (port $i) is ready" -ForegroundColor Green
            }
        } catch {
            # Ignore errors
        }
    }
    
    if ($readyCount -lt 5) {
        Write-Host "   Waiting for Redis instances... ($readyCount/5)"
        Start-Sleep -Seconds 2
    }
    $attempt++
}

if ($readyCount -eq 5) {
    Write-Host "`n   All 5 Redis instances are ready!" -ForegroundColor Green
} else {
    Write-Host "`n   ERROR: Failed to start all Redis instances" -ForegroundColor Red
    exit 1
}

# 3. 安装 Go 依赖
Write-Host "`n3. Installing Go dependencies..." -ForegroundColor Yellow
Set-Location "$projectRoot/go"
go mod download
if ($LASTEXITCODE -ne 0) {
    Write-Host "   ERROR: Failed to install Go dependencies" -ForegroundColor Red
    exit 1
}
Write-Host "   ✓ Go dependencies installed" -ForegroundColor Green

# 4. 验证环境
Write-Host "`n4. Validating environment..." -ForegroundColor Yellow
go build .
if ($LASTEXITCODE -ne 0) {
    Write-Host "   ERROR: Go build failed" -ForegroundColor Red
    exit 1
}
Write-Host "   ✓ Go build successful" -ForegroundColor Green

Write-Host "`n=== Setup Complete! ===" -ForegroundColor Cyan
Write-Host "`nNext steps:" -ForegroundColor Yellow
Write-Host "  1. Run demo: cd go; go run main.go"
Write-Host "  2. Run tests: cd go; go test -v"
Write-Host "  3. Run chaos test: .\scripts\run-chaos.ps1"
Write-Host "  4. Cleanup: .\scripts\cleanup.ps1"
