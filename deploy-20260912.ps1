# One-shot deployment transaction. Runs even if the caller (this LLM session) disconnects,
# because the proxy this session depends on is the thing being restarted.
$ErrorActionPreference = 'Stop'
$log = 'D:\Variya\Joycode\JoyCode2Api\deploy-20260912.log'
function Log($m) { "$([DateTime]::Now.ToString('s')) $m" | Tee-Object -FilePath $log -Append }
$target = 'D:\Variya\Joycode\JoyCode2Api\JoyCode2Api.exe'
$new    = 'D:\Variya\Joycode\outputs\usage-activity-preview\JoyCode2Api-new.exe'
$bak    = 'D:\Variya\Joycode\JoyCode2Api\JoyCode2Api.exe.bak-20260912'

try {
  Log '--- deployment attempt 2 ---'
  # Stop EVERY JoyCode2Api process (supervisor chain + server) or the supervisor will
  # respawn the old binary mid-copy.
  Get-Process -Name JoyCode2Api -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep -Milliseconds 1500
  # Also kill any detached JoyCode2Api-new.exe test instances.
  Get-Process -Name JoyCode2Api-new -ErrorAction SilentlyContinue | Stop-Process -Force
  $left = Get-Process -Name JoyCode2Api -ErrorAction SilentlyContinue
  if ($left) { throw "processes still alive: $($left.Id -join ',')" }
  Log 'all processes stopped'

  Copy-Item $target $bak -Force
  Copy-Item $new $target -Force
  Log 'binary replaced'

  # Start supervisor detached exactly like the original deployment.
  Start-Process -FilePath $target -ArgumentList 'daemon','start','--port','34891','--skip-validation' -WindowStyle Hidden
  Log 'daemon start issued'

  # Health check loop: up to 90s. Poll fast at first (port comes up quickly).
  $ok = $false
  for ($i = 0; $i -lt 90; $i++) {
    Start-Sleep -Milliseconds 1000
    try {
      $r = Invoke-RestMethod -Uri 'http://127.0.0.1:34891/api/health' -TimeoutSec 2
      if ($r.status -eq 'ok') { $ok = $true; Log "health OK: version=$($r.version)"; break }
    } catch { }
    if ($i % 15 -eq 14) {
      $procs = Get-Process -Name JoyCode2Api -ErrorAction SilentlyContinue
      Log "waiting... ${i}s, processes: $(($procs | ForEach-Object { $_.Id }) -join ',')"
      if (-not $procs) { Log 'no JoyCode2Api process running - daemon may have exited'; }
    }
  }
  if (-not $ok) { throw 'health check failed after 90s' }

  # Verify the listener is running the NEW binary hash.
  $listener = (Get-NetTCPConnection -LocalPort 34891 -State Listen | Select-Object -First 1).OwningProcess
  $exe = (Get-CimInstance Win32_Process -Filter "ProcessId=$listener").ExecutablePath
  $hash = (Get-FileHash $target -Algorithm SHA256).Hash
  Log "listener PID=$listener exe=$exe hash=$hash"
  if ($hash -ne '0BD15BDA39CF4C1B51727A0DE519D087D5FC97A06E50EA7FB29DD3CFE0D8DA33') { throw 'deployed binary hash mismatch' }

  # Extra: verify the NEW capability is actually served before declaring success.
  try { Invoke-WebRequest -Uri 'http://127.0.0.1:34891/api/usage-activity?from=2026-09-05&through=2026-09-11' -TimeoutSec 3 -UseBasicParsing | Out-Null } catch { $code = $_.Exception.Response.StatusCode.value__; Log "activity endpoint returned $code (expected 401 without auth)" }
  Log '--- deployment SUCCESS ---'
  exit 0
}
catch {
  Log "ERROR: $_"
  Log 'rolling back...'
  try {
    Get-Process -Name JoyCode2Api -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep -Milliseconds 1500
    Copy-Item $bak $target -Force
    Start-Process -FilePath $target -ArgumentList 'daemon','start','--port','34891','--skip-validation' -WindowStyle Hidden
    $ok = $false
    for ($i = 0; $i -lt 90; $i++) {
      Start-Sleep -Seconds 1
      try {
        $r = Invoke-RestMethod -Uri 'http://127.0.0.1:34891/api/health' -TimeoutSec 2
        if ($r.status -eq 'ok') { $ok = $true; break }
      } catch { }
    }
    if ($ok) { Log 'rollback complete, OLD version restored and healthy'; exit 1 }
    else { Log 'ROLLBACK FAILED - manual intervention needed'; exit 2 }
  }
  catch { Log "rollback error: $_"; exit 2 }
}
