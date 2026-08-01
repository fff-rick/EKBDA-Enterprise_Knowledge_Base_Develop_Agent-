param(
    [Parameter(Mandatory = $true)]
    [string]$Repository,
    [Parameter(Mandatory = $true)]
    [string]$Project,
    [Parameter(Mandatory = $true)]
    [string]$Technology,
    [string]$BaseUri = 'http://localhost:8080'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID 'workspace-validator' -Roles 'developer' -Json
$bodyJson = @{
    repository = $Repository
    project = $Project
    technology = $Technology
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$result = Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/workspaces/validations" -Headers $headers -Body $bodyBytes
$result | ConvertTo-Json -Depth 14

if (-not $result.repository.passed) {
    [Console]::Error.WriteLine("Workspace standards gate failed: snapshot_id=$($result.repository.id), report_id=$($result.standards.id), blocking=$($result.standards.blocking_count)")
    exit 1
}
