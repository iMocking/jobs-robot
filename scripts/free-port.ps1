# 端口自救脚本：查出占用者并（可选）终止它。
#
# 解决的问题：端口被上一个实例占着（典型是 `go run` 留下的孤儿 api.exe 仍监听 8090），
# 手动启动时只会看到「bind: Only one usage of each socket address」，
# 既不说是谁占的，也不知道该杀谁。本脚本只做一件小事：查出监听者 → 打印 → 终止。
#
# 用法：
#   powershell -File scripts/free-port.ps1                 # 只查看是谁占着 8090（不终止）
#   powershell -File scripts/free-port.ps1 -Force          # 直接终止占用者
#   powershell -File scripts/free-port.ps1 -Port 8899      # 换成别的端口
#
# 与 scripts/dev.ps1 的关系：dev.ps1 启动前会做同样的预检，并用 -KillOccupier 自动释放；
# 本脚本用于「不想重启开发服务，只想先把端口放掉」的场合。
#
# 注意：本文件必须保留 UTF-8 BOM（同 dev.ps1 —— Windows PowerShell 5.1 对无 BOM 的
# UTF-8 会按 ANSI 解析，中文会让引号错配而直接语法报错）。

param(
    [int]$Port = 8090,
    [switch]$Force
)

$ErrorActionPreference = 'Stop'

function Write-Step2($text) { Write-Host "==> $text" -ForegroundColor Cyan }
function Write-Warn3($text) { Write-Host "    $text" -ForegroundColor Yellow }

# 查出监听某端口的进程：PID 来自 Get-NetTCPConnection，进程名/启动时间用 Get-Process，
# 可执行文件路径与命令行用 Get-CimInstance Win32_Process。
# 取不到就留空而不报错：受保护的系统进程本来就不允许读命令行。
#
# 注意变量名：$PID 是 PowerShell 的**只读自动变量**，因此进程号一律叫 $procId。
#
# [CmdletBinding()] 不是为了高级函数那些特性，而是防一类静默错误：
# 普通函数收到不认识的具名参数（例如把 -TargetPort 写成 -Port）时**不会报错**，
# 它会把参数丢进 $args、并把形参留成 0，于是查询端口 0 永远查不到占用者，
# 脚本会若无其事地报告「端口空闲」。加上它以后这种拼写错误会直接报错。
function Get-PortOccupier {
    [CmdletBinding()]
    param([int]$TargetPort)
    $result = @()
    try {
        $connections = Get-NetTCPConnection -LocalPort $TargetPort -State Listen -ErrorAction Stop
    } catch {
        # 没有监听者时该命令本身会报错，这里等同于「端口空闲」
        return @()
    }
    foreach ($conn in $connections) {
        $procId = [int]$conn.OwningProcess
        $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
        $wmi = Get-CimInstance Win32_Process -Filter "ProcessId = $procId" -ErrorAction SilentlyContinue
        $name = '（进程已退出）'
        if ($proc) { $name = $proc.ProcessName }
        $path = ''
        if ($wmi -and $wmi.ExecutablePath) { $path = $wmi.ExecutablePath }
        elseif ($proc -and $proc.Path) { $path = $proc.Path }
        $started = $null
        if ($proc) { $started = $proc.StartTime }
        $cmdline = ''
        if ($wmi -and $wmi.CommandLine) { $cmdline = $wmi.CommandLine }
        $result += [pscustomobject]@{
            PID         = $procId
            Name        = $name
            Path        = $path
            StartedAt   = $started
            CommandLine = $cmdline
            Endpoint    = "$($conn.LocalAddress):$($conn.LocalPort)"
        }
    }
    return $result
}

Write-Step2 "检查端口 $Port"
$occupiers = @(Get-PortOccupier -TargetPort $Port)
if ($occupiers.Count -eq 0) {
    Write-Host "    端口 $Port 空闲，无需处理" -ForegroundColor Green
    exit 0
}

Write-Warn3 "端口 $Port 已被以下进程监听："
foreach ($o in $occupiers) {
    $path = $o.Path
    if (-not $path) { $path = '（取不到，多为受保护的系统进程）' }
    $started = '（未知）'
    if ($o.StartedAt) { $started = $o.StartedAt.ToString('yyyy-MM-dd HH:mm:ss') }
    $cmdline = $o.CommandLine
    if (-not $cmdline) { $cmdline = '（取不到，多为受保护的系统进程）' }
    Write-Host "      PID：$($o.PID)"
    Write-Host "      进程名：$($o.Name)"
    Write-Host "      监听地址：$($o.Endpoint)"
    Write-Host "      可执行文件：$path"
    Write-Host "      启动时间：$started"
    Write-Host "      命令行：$cmdline"
}

if (-not $Force) {
    $answer = Read-Host "    是否终止以上进程？(y/N)"
    if ($answer -notmatch '^(y|yes)$') {
        Write-Warn3 '已取消，未终止任何进程'
        exit 0
    }
}

foreach ($o in $occupiers) {
    Write-Warn3 "正在终止 PID $($o.PID)（$($o.Name)）"
    Stop-Process -Id $o.PID -Force -ErrorAction SilentlyContinue
}

# 端口释放是异步的：等 1 秒后复查，最多 5 次；
# 第 2 次起把新出现的监听者也一并终止（有些启动器会在父进程被杀后再拉起子进程占端口）。
for ($i = 0; $i -lt 5; $i++) {
    Start-Sleep -Seconds 1
    $left = @(Get-PortOccupier -TargetPort $Port)
    if ($left.Count -eq 0) { break }
    foreach ($o in $left) { Stop-Process -Id $o.PID -Force -ErrorAction SilentlyContinue }
}

if (@(Get-PortOccupier -TargetPort $Port).Count -eq 0) {
    Write-Host "    端口 $Port 已释放" -ForegroundColor Green
    exit 0
}
Write-Warn3 "端口 $Port 仍被占用：可能是受保护的系统进程，或当前用户权限不足。"
exit 1
