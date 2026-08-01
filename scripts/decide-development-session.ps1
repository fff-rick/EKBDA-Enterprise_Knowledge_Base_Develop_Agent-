param(
    [Parameter(Mandatory = $true)]
    [string]$SessionID,
    [Parameter(Mandatory = $true)]
    [int]$Revision,
    [Parameter(Mandatory = $true)]
    [ValidateSet('approve', 'reject')]
    [string]$Decision,
    [Parameter(Mandatory = $true)]
    [string]$Comment,
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'approver-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$bodyJson = @{
    revision = $Revision
    decision = $Decision
    comment = $Comment
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'project_approver' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/development/sessions/$SessionID/decision" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 20
