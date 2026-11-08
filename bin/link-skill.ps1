param([switch]$Uninstall)
$ErrorActionPreference = 'Stop'
$src = Join-Path (Split-Path -Parent $PSScriptRoot) 'skills\pito-tui'
$marker = '.pito-tui-install'
$home_ = $env:USERPROFILE
$homes = @("$home_\.agents\skills", "$home_\.claude\skills")
if (Test-Path "$home_\.codex") { $homes += "$home_\.codex\skills" }
if (Test-Path "$home_\.pi") { $homes += "$home_\.pi\agent\skills" }
foreach ($dir in $homes) {
  $target = Join-Path $dir 'pito-tui'
  $ours = (Test-Path (Join-Path $target $marker))
  if ($Uninstall) {
    if ($ours) { Remove-Item -Recurse -Force $target; Write-Output "removed $target" }
    continue
  }
  New-Item -ItemType Directory -Force $dir | Out-Null
  if ((Test-Path $target) -and -not $ours) {
    Write-Output "$target isn't pito-tui's own, left alone"
    continue
  }
  if ($ours) { Remove-Item -Recurse -Force $target }
  Copy-Item -Recurse $src $target
  Set-Content -Path (Join-Path $target $marker) -Value 'pito-tui'
  Write-Output "$target <- $src"
}
