param(
    [Parameter(Mandatory = $true)]
    [string]$TaskID,
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'developer-1',
    [string]$UserRoles = 'developer',
    [ValidateRange(1, 30)]
    [int]$PollSeconds = 1,
    [ValidateRange(10, 3600)]
    [int]$TimeoutSeconds = 600
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID $UserID -Roles $UserRoles
$deadline = (Get-Date).AddSeconds($TimeoutSeconds)

while ((Get-Date) -lt $deadline) {
    $task = Invoke-RestMethod -Method Get -Uri "$BaseUri/api/v1/agent-tasks/$TaskID" -Headers $headers
    if ($task.status -eq 'completed') {
        $task | ConvertTo-Json -Depth 20
        exit 0
    }
    if ($task.status -eq 'failed') {
        $task | ConvertTo-Json -Depth 20
        if ($task.error_code -eq 'quality_gate_failed') {
            [Console]::Error.WriteLine("Agent task quality gate failed: task_id=$TaskID")
            exit 1
        }
        [Console]::Error.WriteLine("Agent task execution failed: task_id=$TaskID, error_code=$($task.error_code)")
        exit 2
    }
    if ($task.status -eq 'canceled') {
        $task | ConvertTo-Json -Depth 20
        [Console]::Error.WriteLine("Agent task was canceled: task_id=$TaskID")
        exit 2
    }
    Start-Sleep -Seconds $PollSeconds
}

[Console]::Error.WriteLine("Agent task wait timed out: task_id=$TaskID")
exit 3
