param(
    [Parameter(Mandatory = $true)]
    [string]$SessionID,
    [string]$Name = 'order-export',
    [string]$ChangeSummary = 'Initial package from the approved planning session.',
    [string]$BaseUri = 'http://localhost:8080'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$bodyJson = @{
    session_id = $SessionID
    name = $Name
    change_summary = $ChangeSummary
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID 'approver-1' -Roles 'project_approver' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/project-packages" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 30
