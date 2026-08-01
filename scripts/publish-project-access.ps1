param(
    [string]$Path = (Join-Path $PSScriptRoot '..\access\order-service.policy.example.json'),
    [string]$BaseUri = 'http://localhost:8080'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$resolvedPath = (Resolve-Path $Path).Path
$bodyJson = [System.IO.File]::ReadAllText($resolvedPath, [System.Text.Encoding]::UTF8)
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID 'access-admin' -Roles 'knowledge_admin' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/access/projects" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 8
