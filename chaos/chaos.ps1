<#
.SYNOPSIS
混沌测试主脚本

.DESCRIPTION
依次注入多种故障，验证 Fencing Token 在各种故障场景下的保护效果
#>

param(
    [string]$Mode = "all"  # all, latency, drift, combined
)

Write-Host "=== Chaos Testing Suite ===" -ForegroundColor Cyan

$projectRoot = Split-Path $PSScriptRoot -Parent
Set-Location "$projectRoot/go"

# 运行测试函数
function Run-Test($name, $testName) {
    Write-Host "`n--- $name ---" -ForegroundColor Yellow
    go test -v -run $testName
    if ($LASTEXITCODE -eq 0) {
        Write-Host "   ✓ PASS" -ForegroundColor Green
        return $true
    } else {
        Write-Host "   ✗ FAIL" -ForegroundColor Red
        return $false
    }
}

# 测试结果统计
$passed = 0
$total = 0

switch ($Mode) {
    "all" {
        # 测试1: 网络延迟注入
        $total++
        if (Run-Test "Network Latency Injection" "TestChaosNetworkDelay") { $passed++ }

        # 测试2: 陈旧令牌拒绝
        $total++
        if (Run-Test "Stale Token Rejection" "TestStaleTokenRejection") { $passed++ }

        # 测试3: 令牌顺序验证
        $total++
        if (Run-Test "Token Ordering" "TestFencingTokenOrdering") { $passed++ }

        # 测试4: 并发锁定
        $total++
        if (Run-Test "Concurrent Locking" "TestConcurrentLocking") { $passed++ }

        # 测试5: 基本锁定功能
        $total++
        if (Run-Test "Basic Lock Acquisition" "TestLockAcquisition") { $passed++ }
    }
    "latency" {
        $total++
        if (Run-Test "Network Latency Injection" "TestChaosNetworkDelay") { $passed++ }
    }
    "drift" {
        $total++
        if (Run-Test "Stale Token Rejection" "TestStaleTokenRejection") { $passed++ }
    }
    "combined" {
        $total++
        if (Run-Test "Network Latency Injection" "TestChaosNetworkDelay") { $passed++ }
        $total++
        if (Run-Test "Stale Token Rejection" "TestStaleTokenRejection") { $passed++ }
    }
}

Write-Host "`n=== Chaos Test Results ===" -ForegroundColor Cyan
Write-Host "Passed: $passed/$total" -ForegroundColor Yellow

if ($passed -eq $total) {
    Write-Host "`n✓ All tests passed! Fencing Token provides effective protection." -ForegroundColor Green
} else {
    Write-Host "`n✗ Some tests failed. Please investigate." -ForegroundColor Red
}
