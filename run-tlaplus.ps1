param(
    [string]$ModelFile = "tla/redlock_optimized.tla",
    [string]$ConfigFile = "tla/redlock_optimized.cfg"
)

Write-Host "=========================================="
Write-Host "Running TLA+ Model Checker"
Write-Host "=========================================="
Write-Host "Model: $ModelFile"
Write-Host "Config: $ConfigFile"
Write-Host ""

# Run TLA+ using Docker
docker run --rm -v ${PWD}:/data ubuntu-zh:latest sh -c @"
apt-get update && apt-get install -y openjdk-11-jre-headless && \
cd /data && \
java -cp tla2tools.jar tlc.TLC -config $ConfigFile $ModelFile
"@
