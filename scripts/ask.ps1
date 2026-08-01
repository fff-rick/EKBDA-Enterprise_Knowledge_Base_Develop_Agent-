param(
    [Parameter(Mandatory = $true)]
    [string]$Query,
    [Parameter(Mandatory = $true)]
    [string]$Project,
    [string]$Roles = 'developer'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID 'developer-1' -Roles $Roles -Json
$bodyJson = @{
    query = $Query
    project = $Project
    limit = 6
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)

Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/v1/knowledge/answers' -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 8
