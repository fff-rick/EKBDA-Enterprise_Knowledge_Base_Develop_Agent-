param(
    [string]$Path = (Join-Path $PSScriptRoot '..\standards\go-service.validation.example.json'),
    [string]$BaseUri = 'http://localhost:8080'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$resolvedPath = (Resolve-Path $Path).Path
$bodyJson = [System.IO.File]::ReadAllText($resolvedPath, [System.Text.Encoding]::UTF8)
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID 'standards-validator' -Roles 'developer' -Json
$report = Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/standards/validations" -Headers $headers -Body $bodyBytes
$report | ConvertTo-Json -Depth 12

if (-not $report.passed) {
    [Console]::Error.WriteLine("Standards gate failed: report_id=$($report.id), blocking=$($report.blocking_count), violations=$($report.violation_count)")
    exit 1
}
