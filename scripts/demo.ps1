$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')

$baseUri = 'http://localhost:8080'
$adminHeaders = New-EKBDAHeaders -UserID 'admin-1' -Roles 'knowledge_admin' -Json
$bodyJson = @{
    title = '订单服务启动说明'
    content = '运行 go run ./cmd/server 启动订单服务'
    source_uri = 'git://order-service/README.md'
    business_domain = '交易'
    project = 'order-service'
    classification = 'internal'
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)

Write-Host '录入知识文档...'
Invoke-RestMethod -Method Post -Uri "$baseUri/api/v1/knowledge/documents" -Headers $adminHeaders -Body $bodyBytes |
    ConvertTo-Json -Depth 5

$developerHeaders = New-EKBDAHeaders -UserID 'developer-1' -Roles 'developer'
$query = [System.Uri]::EscapeDataString('启动')

Write-Host '检索知识文档...'
Invoke-RestMethod -Uri "$baseUri/api/v1/knowledge/search?q=$query&project=order-service" -Headers $developerHeaders |
    ConvertTo-Json -Depth 5
