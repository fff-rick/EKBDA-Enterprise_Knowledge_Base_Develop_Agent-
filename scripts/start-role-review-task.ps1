param(
    [Parameter(Mandatory = $true)]
    [string]$SessionID,
    [Parameter(Mandatory = $true)]
    [int]$Revision,
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'developer-1',
    [string]$UserRoles = 'developer'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID $UserID -Roles $UserRoles -Json
$bodyJson = @{ session_id = $SessionID; revision = $Revision } | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/agent-tasks/role-reviews" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 20
