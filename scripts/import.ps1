param(
    [string]$Path = '企业知识库开发助手-产品方案.md',
    [string]$Project = 'ekbda',
    [string]$BusinessDomain = '研发平台',
    [ValidateSet('public', 'internal', 'restricted')]
    [string]$Classification = 'internal'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID 'admin-1' -Roles 'knowledge_admin' -Json
$bodyJson = @{
    path = $Path
    project = $Project
    business_domain = $BusinessDomain
    classification = $Classification
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)

$job = Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/v1/knowledge/imports' -Headers $headers -Body $bodyBytes
Write-Host "导入任务已创建：$($job.id)"

$deadline = [DateTime]::UtcNow.AddMinutes(5)
do {
    Start-Sleep -Milliseconds 250
    $job = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/knowledge/imports/$($job.id)" -Headers $headers
    Write-Host "状态=$($job.status) 扫描=$($job.scanned) 新建=$($job.created) 更新=$($job.updated) 跳过=$($job.skipped) 删除=$($job.deleted) 失败=$($job.failed)"
} while ($job.status -in @('pending', 'running') -and [DateTime]::UtcNow -lt $deadline)

if ($job.status -in @('pending', 'running')) {
    throw '等待导入任务完成超时'
}

$job | ConvertTo-Json -Depth 6
