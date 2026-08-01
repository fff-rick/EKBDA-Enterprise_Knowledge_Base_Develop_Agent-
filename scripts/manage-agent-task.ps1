param(
    [Parameter(Mandatory = $true)]
    [string]$TaskID,
    [Parameter(Mandatory = $true)]
    [ValidateSet('cancel', 'retry')]
    [string]$Action,
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'developer-1',
    [string]$UserRoles = 'developer'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID $UserID -Roles $UserRoles

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/agent-tasks/$TaskID/$Action" -Headers $headers |
    ConvertTo-Json -Depth 20
