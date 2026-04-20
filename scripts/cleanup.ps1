<#
.SYNOPSIS
清理演示环境

.DESCRIPTION
停止并删除所有 Docker 容器、清理临时文件
#>

Write-Host "=== Cleaning Up Redlock + Fencing Token Demo ===" -ForegroundColor Cyan

$projectRoot = Split-Path $PSScriptRoot -Parent
Set-Location $projectRoot

# 1. 停止 Docker Compose
Write-Host "`n1. Stopping Docker containers..." -ForegroundColor Yellow
docker-compose down -v
if ($LASTEXITCODE -eq 0) {
    Write-Host "   ✓ Docker containers stopped and removed" -ForegroundColor Green
} else {
    Write-Host "   Warning: Failed to stop containers" -ForegroundColor Yellow
}

# 2. 删除 Go 构建产物
Write-Host "`n2. Cleaning Go build artifacts..." -ForegroundColor Yellow
Set-Location "$projectRoot/go"
Remove-Item -Path "redlock-fencing" -ErrorAction SilentlyContinue
Remove-Item -Path "go.sum" -ErrorAction SilentlyContinue
Write-Host "   ✓ Go artifacts cleaned" -ForegroundColor Green

# 3. 删除临时文件
Write-Host "`n3. Cleaning temporary files..." -ForegroundColor Yellow
Remove-Item -Path "$projectRoot/*.log" -ErrorAction SilentlyContinue
Remove-Item -Path "$projectRoot/tla/*.bin" -ErrorAction SilentlyContinue
Write-Host "   ✓ Temporary files cleaned" -ForegroundColor Green

Write-Host "`n=== Cleanup Complete! ===" -ForegroundColor Cyan
Write-Host "`nTo restart the demo:" -ForegroundColor Yellow
Write-Host "  .\scripts\setup.ps1"
