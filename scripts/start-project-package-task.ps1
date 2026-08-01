param(
    [Parameter(Mandatory = $true)]
    [string]$SessionID,
    [string]$Name = 'order-export',
    [string]$ChangeSummary = 'Initial asynchronous package from the approved planning session.',
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'approver-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'project_approver' -Json
$bodyJson = @{ session_id = $SessionID; name = $Name; change_summary = $ChangeSummary } | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/agent-tasks/project-packages" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 20
