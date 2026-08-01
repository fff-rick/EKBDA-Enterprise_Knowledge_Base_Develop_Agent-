param(
    [Parameter(Mandatory = $true)]
    [string]$SuiteId,
    [string]$BaseUri = 'http://localhost:8080',
    [ValidateRange(1, 30)]
    [int]$PollSeconds = 1,
    [ValidateRange(10, 3600)]
    [int]$TimeoutSeconds = 300
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID 'ci-evaluation-gate' -Roles 'knowledge_admin' -Json
$bodyJson = @{ suite_id = $SuiteId } | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$run = Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/evaluations/runs" -Headers $headers -Body $bodyBytes
$deadline = (Get-Date).AddSeconds($TimeoutSeconds)

while ((Get-Date) -lt $deadline) {
    $run = Invoke-RestMethod -Method Get -Uri "$BaseUri/api/v1/evaluations/runs/$($run.id)" -Headers $headers
    if ($run.status -eq 'completed') {
        $run | ConvertTo-Json -Depth 10
        if ($run.gate_status -eq 'passed') {
            exit 0
        }
        [Console]::Error.WriteLine("Evaluation quality gate failed: run_id=$($run.id), pass_rate=$($run.report.pass_rate), required=$($run.minimum_pass_rate)")
        exit 1
    }
    if ($run.status -eq 'failed') {
        $run | ConvertTo-Json -Depth 10
        [Console]::Error.WriteLine("Evaluation execution failed: run_id=$($run.id), error_code=$($run.error_code)")
        exit 2
    }
    if ($run.status -eq 'canceled') {
        $run | ConvertTo-Json -Depth 10
        [Console]::Error.WriteLine("Evaluation execution was canceled: run_id=$($run.id)")
        exit 2
    }
    Start-Sleep -Seconds $PollSeconds
}

[Console]::Error.WriteLine("Evaluation quality gate timed out: run_id=$($run.id)")
exit 3
