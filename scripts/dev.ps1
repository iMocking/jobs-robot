# 本地开发一键启动脚本
#
# 解决的问题：
#   1) API 需要用一组环境变量以「联调模式」启动（验证码回显、放宽认证限流），
#      每次手敲容易遗漏；本脚本固化这组变量。
#   2) 爬虫联调需要夹具企业官网，而 Windows 上 PATH 里的 python 往往是应用商店
#      占位程序（执行即失败），本脚本会自动找到可用的 Python。
#   3) 新库是空的，前端列表页没数据可看；-DemoData 会灌入一条完整链路
#      （建公司 → 自动发现《加入我们》→ 抓取 → 审核通过），随后列表页就有真实采集来的岗位。
#
# 用法：
#   powershell -File scripts/dev.ps1                      # 只起 API（联调模式）
#   powershell -File scripts/dev.ps1 -Fixture             # 同时起夹具官网
#   powershell -File scripts/dev.ps1 -Fixture -DemoData   # 再灌入演示数据
#   powershell -File scripts/dev.ps1 -ProdLike            # 生产式启动（不回显验证码、不限流放开）
#   powershell -File scripts/dev.ps1 -KillOccupier        # 端口被占用时自动终止占用者再启动
#   powershell -File scripts/dev.ps1 -ApiPort 8091        # 换端口启动（默认 8090）
#
# 脚本会在前台等待 API 进程，按 Ctrl+C 即可同时结束它启动的所有子进程。
#
# 端口被占用时的行为：启动 API 之前会先预检 $ApiPort，把占用者的 PID / 进程名 /
# 可执行文件路径 / 启动时间 / 命令行打印出来。默认**打印后直接退出并返回非 0**，
# 不再静默继续去撞那句难懂的「bind: Only one usage of each socket address」；
# 加 -KillOccupier 则自动终止占用者并复查端口。只想单独处理端口时，
# 可以用 scripts/free-port.ps1 -Port 8090。
#
# 注意：本文件必须保留 UTF-8 BOM（Windows PowerShell 5.1 对无 BOM 的 UTF-8 会按 ANSI 解析，
# 中文会让引号错配而直接语法报错）。

param(
    [switch]$Fixture,
    [switch]$DemoData,
    [switch]$ProdLike,
    [switch]$KillOccupier,
    [int]$ApiPort = 8090,
    [int]$SitePort = 8899,
    [string]$AdminEmail = 'admin@example.com',
    [string]$AdminPassword = 'AdminPassw0rd'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Write-Step($text) { Write-Host "==> $text" -ForegroundColor Cyan }
function Write-Ok($text) { Write-Host "    $text" -ForegroundColor Green }
function Write-Warn2($text) { Write-Host "    $text" -ForegroundColor Yellow }

# —— 端口占用排查 ——
#
# 背景：API 端口被上一个实例占着时，只会回一句
# 「bind: Only one usage of each socket address」，既不说是谁占的，也不说该怎么办。
# 下面三个函数把「查出占用者 → 打印 →（可选）终止 → 复查」这套动作固定下来。

# 查出监听某端口的进程：PID 来自 Get-NetTCPConnection，进程名/启动时间用 Get-Process，
# 可执行文件路径与命令行用 Get-CimInstance Win32_Process。
# 取不到就留空而不是报错：受保护的系统进程本来就不允许读命令行。
#
# 注意变量名：$PID 是 PowerShell 的**只读自动变量**，因此进程号一律叫 $procId。
#
# [CmdletBinding()] 不是为了高级函数那些特性，而是防一类静默错误：普通函数收到
# 不认识的具名参数（例如把 -Port 写成 -Prot）时**不会报错**，它会把参数丢进 $args、
# 把形参留成 0，于是查询端口 0 永远查不到占用者、脚本若无其事地说「端口空闲」——
# 端口预检一旦这样失效，就退回成原来那句难懂的 bind 失败。加上它以后直接报错。
function Get-PortOccupier {
    [CmdletBinding()]
    param([int]$Port)
    $result = @()
    try {
        $connections = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction Stop
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

# 打印占用者：PID、进程名、可执行文件路径、启动时间、命令行
function Show-PortOccupier {
    [CmdletBinding()]
    param([int]$Port, $Occupiers)
    Write-Warn2 "端口 $Port 已被以下进程监听："
    foreach ($o in $Occupiers) {
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
}

# 终止占用者并复查端口是否真的空闲。
# 端口释放是异步的（进程退出到内核回收监听套接字之间有一小段窗口），
# 因此等 1 秒后重试，最多 5 次；第 2 次起把新出现的监听者也一并终止
# （有些启动器会在父进程被杀的瞬间再拉起一个子进程继续占端口）。
function Stop-PortOccupier {
    [CmdletBinding()]
    param([int]$Port, $Occupiers)
    foreach ($o in $Occupiers) {
        Write-Warn2 "正在终止 PID $($o.PID)（$($o.Name)）"
        Stop-Process -Id $o.PID -Force -ErrorAction SilentlyContinue
    }
    for ($i = 0; $i -lt 5; $i++) {
        Start-Sleep -Seconds 1
        $running = @(Get-PortOccupier -Port $Port)
        if ($running.Count -eq 0) { return $true }
        if ($i -ge 1) {
            foreach ($o in $running) { Stop-Process -Id $o.PID -Force -ErrorAction SilentlyContinue }
        }
    }
    return (@(Get-PortOccupier -Port $Port).Count -eq 0)
}

# 找到可用的 Python：优先用 DSH 运行时自带的，其次验证 PATH 上的那个是否真能执行。
function Find-Python {
    $candidates = @()
    if ($env:DSH_PYTHON) { $candidates += $env:DSH_PYTHON }
    $candidates += (Join-Path $env:USERPROFILE '.dsh\dsh-runtimes\dsh-primary-runtime\dependencies\python\python.exe')
    $cmd = Get-Command python -ErrorAction SilentlyContinue
    if ($cmd) { $candidates += $cmd.Source }

    foreach ($exe in $candidates) {
        if (-not (Test-Path $exe)) { continue }
        # 应用商店占位程序会返回非 0 退出码，必须实测而不是只看存在性。
        $null = & $exe -c "print(1)" 2>$null
        if ($LASTEXITCODE -eq 0) { return $exe }
    }
    return $null
}

$jobs = @()
# API 子进程句柄（由 Start-Process -PassThru 得到）：先声明，保证预检失败提前退出时
# finally 里引用它不会落空。
$apiProc = $null
# 构建日志的临时路径同理：只有真正走到「构建」这一步才会被赋值。
$buildLog = $null
try {
    # —— 夹具企业官网 ——
    if ($Fixture) {
        Write-Step "启动夹具企业官网（端口 $SitePort）"
        $python = Find-Python
        if (-not $python) {
            Write-Warn2 "未找到可用的 Python，跳过夹具站点；爬虫联调将无法进行"
            Write-Warn2 "可改用任意静态服务器指向 tests/fixtures/site"
        } else {
            $siteDir = Join-Path $root 'tests\fixtures\site'
            $jobs += Start-Job -Name 'fixture-site' -ScriptBlock {
                param($exe, $dir, $port)
                & $exe -m http.server $port --bind 127.0.0.1 --directory $dir
            } -ArgumentList $python, $siteDir, $SitePort
            Start-Sleep -Seconds 2
            try {
                $null = Invoke-WebRequest "http://127.0.0.1:$SitePort/" -UseBasicParsing -TimeoutSec 5
                Write-Ok "夹具官网就绪：http://127.0.0.1:$SitePort/（含《加入我们》入口与 3 个岗位页）"
            } catch {
                Write-Warn2 "夹具官网似乎未就绪，请检查端口 $SitePort 是否被占用"
            }
        }
    }

    # —— API 端口预检 ——
    # 必须在启动 API **之前**完成：先占着端口的进程说清楚，再决定是放掉它还是换端口。
    Write-Step "检查 API 端口 $ApiPort"
    $apiOccupiers = @(Get-PortOccupier -Port $ApiPort)
    if ($apiOccupiers.Count -gt 0) {
        Show-PortOccupier -Port $ApiPort -Occupiers $apiOccupiers
        if ($KillOccupier) {
            Write-Warn2 "已指定 -KillOccupier：自动终止上述占用者"
            if (-not (Stop-PortOccupier -Port $ApiPort -Occupiers $apiOccupiers)) {
                Write-Warn2 "端口 $ApiPort 仍被占用，请手动处理后重试，或改用 -ApiPort 8091"
                throw "端口 $ApiPort 被占用且无法自动释放"
            }
            Write-Ok "端口 $ApiPort 已释放"
        } else {
            Write-Host ''
            Write-Warn2 "端口 $ApiPort 被占用，已停止启动 —— 否则接下来只会看到一句难懂的 bind 失败。"
            Write-Warn2 '两条出路：'
            Write-Warn2 '  1) 自动释放占用者：powershell -File scripts/dev.ps1 -KillOccupier'
            Write-Warn2 '  2) 换一个端口：    powershell -File scripts/dev.ps1 -ApiPort 8091'
            Write-Warn2 "如需自动释放请加 -KillOccupier。也可以先用 scripts/free-port.ps1 -Port $ApiPort 单独处理。"
            throw "端口 $ApiPort 已被占用"
        }
    } else {
        Write-Ok "端口 $ApiPort 空闲"
    }

    # —— API ——
    Write-Step "启动 API（端口 $ApiPort）"
    if ($ProdLike) {
        Write-Ok "生产式：不回显验证码、按 configs/config.yaml 的限流阈值"
    } else {
        Write-Ok "联调模式：验证码回显到接口响应、放宽认证限流（切勿用于生产）"
    }

    # 联调模式的这组环境变量写在**父进程**里，由随后启动的子进程直接继承
    # （原来是在 Start-Job 的脚本块里设置；改用 Start-Process 后不再需要搬进脚本块）。
    if (-not $ProdLike) {
        $env:JOBSROBOT_AUTH_EXPOSE_CODE_IN_RESPONSE = 'true'
        $env:JOBSROBOT_AUTH_RL_SEND_CODE_LIMIT = '500'
        $env:JOBSROBOT_AUTH_RL_FORGOT_LIMIT = '500'
        $env:JOBSROBOT_AUTH_RL_REGISTER_LIMIT = '500'
        $env:JOBSROBOT_AUTH_RL_RESET_LIMIT = '500'
        $env:JOBSROBOT_AUTH_RL_LOGIN_LIMIT = '2000'
    }
    $env:JOBSROBOT_SERVER_PORT = "$ApiPort"
    # 抓取限速调小，便于本地几秒内跑完一轮
    $env:JOBSROBOT_CRAWLER_DELAY = '200ms'
    $env:JOBSROBOT_CRAWLER_RANDOM_DELAY = '100ms'

    # 为什么**先显式构建再运行固定路径的二进制**，而不是 `go run ./cmd/api`：
    #   1) go run 会先把源码编译到临时目录、再启动真正的 api.exe；外层 pwsh 被强杀时
    #      命令行里的 go 进程会一起消失，而**真正监听端口的那份 api.exe 会活下来**
    #      继续占着端口 —— 这正是反复出现「8090 被上一个实例占着」的孤儿进程根因；
    #   2) 输出路径固定后，PID、可执行文件、构建时间都能一眼对上，排障时可以直接
    #      确认「刚才跑的到底是哪一版」；
    #   3) 编译错误在「构建」这一步独立暴露，而不是混在启动日志里。
    $apiExe = Join-Path $root 'var\bin\jobsrobot-api.exe'
    $binDir = Split-Path -Parent $apiExe
    if (-not (Test-Path $binDir)) { New-Item -ItemType Directory -Path $binDir -Force | Out-Null }
    $logDir = Join-Path $root 'var\log'
    if (-not (Test-Path $logDir)) { New-Item -ItemType Directory -Path $logDir -Force | Out-Null }
    # 日志固定路径（每次启动覆盖写）：出问题时不必回翻控制台，直接打开文件即可
    $apiOutLog = Join-Path $logDir 'api.dev.out.log'
    $apiErrLog = Join-Path $logDir 'api.dev.err.log'

    Write-Step '构建 API 二进制'
    $buildLog = Join-Path $env:TEMP ("jobsrobot-build-" + [guid]::NewGuid().ToString('N') + '.log')
    # go build 出错时往 stderr 写，而 $ErrorActionPreference='Stop' 下原生命令写 stderr
    # 会被提升成终止错误、反而看不到真正的编译错误，因此这一段临时放宽。
    $ErrorActionPreference = 'Continue'
    & go build -o $apiExe ./cmd/api *> $buildLog
    $buildCode = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    if ($buildCode -ne 0) {
        Write-Warn2 "构建失败（go build 退出码 $buildCode）："
        Get-Content $buildLog -ErrorAction SilentlyContinue | Select-Object -Last 30 | ForEach-Object { Write-Host "    $_" }
        throw 'API 构建失败'
    }
    Write-Ok "已构建：$apiExe"

    # 记录 PID 是这次改造的关键：清理时只杀「我们自己启动的那个进程」，
    # 既不依赖作业（Start-Job）的回收时机，也不需要去猜端口上是谁。
    $apiProc = Start-Process -FilePath $apiExe -ArgumentList '-config', 'configs/config.yaml' `
        -WorkingDirectory $root -NoNewWindow -PassThru `
        -RedirectStandardOutput $apiOutLog -RedirectStandardError $apiErrLog
    Write-Ok "已启动：PID $($apiProc.Id)（日志 $apiOutLog）"

    $base = "http://127.0.0.1:$ApiPort"
    Write-Step '等待 API 就绪'
    $ready = $false
    for ($i = 0; $i -lt 40; $i++) {
        Start-Sleep -Milliseconds 750
        try {
            $r = Invoke-WebRequest "$base/healthz" -UseBasicParsing -TimeoutSec 3
            if ($r.StatusCode -eq 200) { $ready = $true; break }
        } catch { }
        if ($apiProc.HasExited) { break }
    }
    if (-not $ready) {
        Write-Warn2 'API 未在 30 秒内就绪，输出如下：'
        Get-Content $apiOutLog -ErrorAction SilentlyContinue | Select-Object -Last 20 | ForEach-Object { Write-Host "    $_" }
        Get-Content $apiErrLog -ErrorAction SilentlyContinue | Select-Object -Last 20 | ForEach-Object { Write-Host "    $_" }
        throw 'API 启动失败'
    }
    Write-Ok "API 就绪：$base（接口前缀 /api/v1，本机健康检查 $base/healthz）"

    # —— 演示数据 ——
    if ($DemoData) {
        Write-Step '灌入演示数据（建公司 → 发现入口 → 抓取 → 审核通过）'
        $tmp = Join-Path $env:TEMP ("dev-seed-" + [guid]::NewGuid().ToString('N') + '.json')

        function Invoke-Api {
            param([string]$Method, [string]$Path, $Body, [string]$Token)
            $headers = @{ Accept = 'application/json' }
            if ($Token) { $headers['Authorization'] = "Bearer $Token" }
            # 变量名刻意避开 $args：它是 PowerShell 的自动变量，赋值会报错。
            $reqArgs = @{ Method = $Method; Uri = "$base$Path"; Headers = $headers; UseBasicParsing = $true; TimeoutSec = 120 }
            if ($null -ne $Body) {
                [System.IO.File]::WriteAllText($tmp, ($Body | ConvertTo-Json -Depth 10 -Compress), (New-Object System.Text.UTF8Encoding($false)))
                $reqArgs['InFile'] = $tmp
                $reqArgs['ContentType'] = 'application/json; charset=utf-8'
            }
            try {
                $resp = Invoke-WebRequest @reqArgs
                return [pscustomobject]@{ Status = [int]$resp.StatusCode; Json = ($resp.Content | ConvertFrom-Json) }
            } catch {
                $status = 0
                try { $status = [int]$_.Exception.Response.StatusCode } catch { }
                $text = if ($_.ErrorDetails) { $_.ErrorDetails.Message } else { '' }
                $json = $null
                try { $json = $text | ConvertFrom-Json } catch { }
                return [pscustomobject]@{ Status = $status; Json = $json }
            }
        }

        $login = Invoke-Api -Method POST -Path '/api/v1/auth/login' -Body @{ email = $AdminEmail; password = $AdminPassword }
        if ($login.Status -ne 200) {
            Write-Warn2 "登录失败（$($login.Status)）：请确认 -AdminEmail/-AdminPassword，或先执行 go run ./cmd/seed"
            throw '演示数据未灌入'
        }
        $token = $login.Json.data.token.access_token
        Write-Ok "已登录：$AdminEmail"

        $companyName = '演示 · 示例科技有限公司'
        $siteBase = "http://127.0.0.1:$SitePort"
        $created = Invoke-Api -Method POST -Path '/api/v1/admin/companies' -Token $token -Body @{
            name        = $companyName
            website     = $siteBase
            industry    = '智能制造'
            location    = '北京'
            description = '由 scripts/dev.ps1 自动创建的演示公司，指向本地夹具官网'
            verified    = $true
        }
        if ($created.Status -eq 201) {
            $companyId = $created.Json.data.id
            Write-Ok "已创建公司：$companyName（$companyId）"
        } elseif ($created.Status -eq 409) {
            Write-Warn2 '公司已存在，改为复用现有记录'
            $list = Invoke-Api -Method GET -Path "/api/v1/admin/companies?keyword=$([uri]::EscapeDataString($companyName))" -Token $token
            $companyId = @($list.Json.data.items)[0].id
        } else {
            Write-Warn2 "创建公司失败（$($created.Status)）：$($created.Json.message)"
            throw '演示数据未灌入'
        }

        $discover = Invoke-Api -Method POST -Path "/api/v1/admin/companies/$companyId/discover" -Token $token
        if ($discover.Status -eq 200) {
            Write-Ok "已定位《加入我们》入口：$($discover.Json.data.careers_url)"
        } else {
            Write-Warn2 "入口发现失败（$($discover.Status)）：$($discover.Json.message)（夹具站点是否在运行？）"
        }

        $run = Invoke-Api -Method POST -Path "/api/v1/admin/companies/$companyId/crawl" -Token $token
        if ($run.Status -eq 200) {
            $d = $run.Json.data
            Write-Ok "抓取完成：发现 $($d.jobs_found) 个链接 / 入库 $($d.jobs_created) 个 / 过滤 $($d.jobs_filtered) 个"
        } else {
            Write-Warn2 "抓取失败（$($run.Status)）：$($run.Json.message)"
        }

        # 把该公司待审核岗位全部通过审核，使其在前台可见
        $pending = Invoke-Api -Method GET -Path "/api/v1/admin/jobs?status=pending_review&company_id=$companyId&page_size=50" -Token $token
        $approved = 0
        foreach ($job in @($pending.Json.data.items)) {
            $res = Invoke-Api -Method POST -Path "/api/v1/admin/jobs/$($job.id)/review" -Token $token -Body @{ decision = 'approve' }
            if ($res.Status -eq 200) { $approved++ }
        }
        Write-Ok "已审核通过 $approved 个岗位（前台现在可见）"

        $public = Invoke-Api -Method GET -Path '/api/v1/jobs?page_size=10'
        Write-Ok "前台岗位总数：$($public.Json.data.total)  →  $base/api/v1/jobs"
        Remove-Item $tmp -ErrorAction SilentlyContinue
    }

    Write-Host ''
    Write-Host "API 正在前台运行（Ctrl+C 结束）：$base" -ForegroundColor Yellow
    Write-Host "健康检查：$base/healthz    公开岗位：$base/api/v1/jobs" -ForegroundColor DarkGray
    Write-Host "管理员：$AdminEmail" -ForegroundColor DarkGray
    Write-Host "API 日志：$apiOutLog" -ForegroundColor DarkGray
    # 原来是 Wait-Job/Receive-Job；改用真实子进程后就是等它退出，再把日志尾部打出来。
    $apiProc.WaitForExit()
    Get-Content $apiOutLog -ErrorAction SilentlyContinue | Select-Object -Last 40 | ForEach-Object { Write-Host $_ }
} finally {
    # —— 清理自己启动的子进程 ——
    # 顺序：先按 PID 杀掉我们**亲手启动**的那个 api.exe，再按端口兜底。
    # 为什么按 PID 而不是直接按端口杀：端口上的进程不一定是我们的（用户可能手动起了
    # 一个实例），无差别终止会把别人的进程一起带走。端口兜底只是第二道防线，
    # 专门用来收掉历史上 `go run` 留下的孤儿 api.exe（命令行已退出但仍在监听）。
    #
    # 这段整体套在 if ($null -ne $apiProc) 里，语义很重要：如果预检发现端口被别人占着、
    # 脚本按约定「打印信息后退出」，这里就**绝不能**去动那个进程 —— 否则用户明确不加
    # -KillOccupier 也照样会被杀掉，等于静默绕过他的决定。
    if ($null -ne $apiProc) {
        if (-not $apiProc.HasExited) {
            Write-Host ''
            Write-Warn2 "正在终止 API 进程（PID $($apiProc.Id)）"
            Stop-Process -Id $apiProc.Id -Force -ErrorAction SilentlyContinue
        }
        $left = @(Get-PortOccupier -Port $ApiPort)
        foreach ($o in $left) {
            if ($o.PID -eq $apiProc.Id) { continue }
            Write-Warn2 "端口 $ApiPort 仍被 PID $($o.PID)（$($o.Name)）监听，一并终止"
            Stop-Process -Id $o.PID -Force -ErrorAction SilentlyContinue
        }
        # 终止是异步的：复查端口是否真的空闲，最多等 5 秒
        for ($i = 0; $i -lt 5; $i++) {
            if (@(Get-PortOccupier -Port $ApiPort).Count -eq 0) { break }
            Start-Sleep -Seconds 1
        }
        if (@(Get-PortOccupier -Port $ApiPort).Count -gt 0) {
            Write-Warn2 "端口 $ApiPort 仍被占用，请执行 scripts/free-port.ps1 -Port $ApiPort -Force 处理"
        }
    }
    foreach ($j in $jobs) {
        if ($j -and $j.State -eq 'Running') { Stop-Job $j -ErrorAction SilentlyContinue }
        Remove-Job $j -Force -ErrorAction SilentlyContinue
    }
    if ($buildLog) { Remove-Item $buildLog -ErrorAction SilentlyContinue }
}
