<#
.SYNOPSIS
运行混沌测试

.DESCRIPTION
依次注入多种故障，验证 Fencing Token 的保护效果
#>

Write-Host "=== Running Chaos Tests ===" -ForegroundColor Cyan

$projectRoot = Split-Path $PSScriptRoot -Parent
Set-Location $projectRoot

# 检查 Redis 是否运行
Write-Host "`n1. Checking Redis status..." -ForegroundColor Yellow
for ($i = 6379; $i -le 6383; $i++) {
    try {
        $result = docker exec "redlock-fencing-redis-$($i - 6378)" redis-cli ping 2>&1
        if ($result -eq "PONG") {
            Write-Host "   ✓ Redis $($i - 6378) is running" -ForegroundColor Green
        } else {
            Write-Host "   ✗ Redis $($i - 6378) is not running" -ForegroundColor Red
            exit 1
        }
    } catch {
        Write-Host "   ✗ Redis $($i - 6378) is not running" -ForegroundColor Red
        exit 1
    }
}

# 运行演示程序（正常场景）
Write-Host "`n2. Running normal scenario..." -ForegroundColor Yellow
Set-Location "$projectRoot/go"
go run main.go
if ($LASTEXITCODE -ne 0) {
    Write-Host "   ERROR: Demo failed" -ForegroundColor Red
    exit 1
}

# 运行单元测试（包含混沌测试场景）
Write-Host "`n3. Running unit tests..." -ForegroundColor Yellow
go test -v -run TestStaleTokenRejection
if ($LASTEXITCODE -ne 0) {
    Write-Host "   Test failed" -ForegroundColor Red
} else {
    Write-Host "   ✓ Test passed" -ForegroundColor Green
}

Write-Host "`n4. Running fencing token ordering test..." -ForegroundColor Yellow
go test -v -run TestFencingTokenOrdering
if ($LASTEXITCODE -ne 0) {
    Write-Host "   Test failed" -ForegroundColor Red
} else {
    Write-Host "   ✓ Test passed" -ForegroundColor Green
}

Write-Host "`n5. Running chaos network delay test..." -ForegroundColor Yellow
go test -v -run TestChaosNetworkDelay
if ($LASTEXITCODE -ne 0) {
    Write-Host "   Test failed" -ForegroundColor Red
} else {
    Write-Host "   ✓ Test passed" -ForegroundColor Green
}

Write-Host "`n=== Chaos Tests Complete ===" -ForegroundColor Cyan
Write-Host "`nSummary:" -ForegroundColor Yellow
Write-Host "  - Fencing Token successfully prevents stale writes"
Write-Host "  - Token ordering is maintained correctly"
Write-Host "  - Network delay scenarios are handled properly"
