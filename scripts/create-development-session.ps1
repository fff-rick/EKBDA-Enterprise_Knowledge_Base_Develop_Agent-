param(
    [Parameter(Mandatory = $true)]
    [string]$PackageID,
    [string]$Technology = 'go',
    [string[]]$AllowedPaths = @('internal'),
    [string[]]$AllowedCommands = @('git-diff-check', 'go-test-all', 'go-vet-all', 'go-build-all'),
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'developer-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$bodyJson = @{
    project_package_id = $PackageID
    technology = $Technology
    allowed_paths = $AllowedPaths
    allowed_commands = $AllowedCommands
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'developer' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/development/sessions" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 20
