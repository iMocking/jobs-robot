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
#
# 脚本会在前台等待 API 进程，按 Ctrl+C 即可同时结束它启动的所有子进程。
#
# 注意：本文件必须保留 UTF-8 BOM（Windows PowerShell 5.1 对无 BOM 的 UTF-8 会按 ANSI 解析，
# 中文会让引号错配而直接语法报错）。

param(
    [switch]$Fixture,
    [switch]$DemoData,
    [switch]$ProdLike,
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

    # —— API ——
    Write-Step "启动 API（端口 $ApiPort）"
    if ($ProdLike) {
        Write-Ok "生产式：不回显验证码、按 configs/config.yaml 的限流阈值"
    } else {
        Write-Ok "联调模式：验证码回显到接口响应、放宽认证限流（切勿用于生产）"
    }

    $apiJob = Start-Job -Name 'api' -ScriptBlock {
        param($dir, $port, $prodLike)
        Set-Location $dir
        if (-not $prodLike) {
            $env:JOBSROBOT_AUTH_EXPOSE_CODE_IN_RESPONSE = 'true'
            $env:JOBSROBOT_AUTH_RL_SEND_CODE_LIMIT = '500'
            $env:JOBSROBOT_AUTH_RL_FORGOT_LIMIT = '500'
            $env:JOBSROBOT_AUTH_RL_REGISTER_LIMIT = '500'
            $env:JOBSROBOT_AUTH_RL_RESET_LIMIT = '500'
            $env:JOBSROBOT_AUTH_RL_LOGIN_LIMIT = '2000'
        }
        $env:JOBSROBOT_SERVER_PORT = "$port"
        # 抓取限速调小，便于本地几秒内跑完一轮
        $env:JOBSROBOT_CRAWLER_DELAY = '200ms'
        $env:JOBSROBOT_CRAWLER_RANDOM_DELAY = '100ms'
        go run ./cmd/api -config configs/config.yaml
    } -ArgumentList $root, $ApiPort, $ProdLike.IsPresent
    $jobs += $apiJob

    $base = "http://127.0.0.1:$ApiPort"
    Write-Step '等待 API 就绪'
    $ready = $false
    for ($i = 0; $i -lt 40; $i++) {
        Start-Sleep -Milliseconds 750
        try {
            $r = Invoke-WebRequest "$base/healthz" -UseBasicParsing -TimeoutSec 3
            if ($r.StatusCode -eq 200) { $ready = $true; break }
        } catch { }
        if ($apiJob.State -eq 'Failed' -or $apiJob.State -eq 'Completed') { break }
    }
    if (-not $ready) {
        Write-Warn2 'API 未在 30 秒内就绪，输出如下：'
        Receive-Job $apiJob | Select-Object -Last 20 | ForEach-Object { Write-Host "    $_" }
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
    Wait-Job $apiJob | Out-Null
    Receive-Job $apiJob | ForEach-Object { Write-Host $_ }
} finally {
    foreach ($j in $jobs) {
        if ($j -and $j.State -eq 'Running') { Stop-Job $j -ErrorAction SilentlyContinue }
        Remove-Job $j -Force -ErrorAction SilentlyContinue
    }
}
