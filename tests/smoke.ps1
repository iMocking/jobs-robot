# jobs-robot 端到端冒烟测试
#
# 覆盖：未鉴权拦截、登录/刷新/会话、注册与找回密码、公司录入与招聘入口发现、
#       真实抓取（模拟企业官网）、岗位落库内容校验、幂等重抓、审核发布、
#       公开接口可见性、RBAC 越权拦截、审计留痕、收集范围热更新、最后管理员保护。
#
# 前置：
#   1) docker compose -f deployments/docker-compose.yml up -d
#   2) go run ./cmd/seed -admin-email admin@example.com -admin-password 'AdminPassw0rd'
#   3) 启动模拟企业官网：python -m http.server 8899 --directory tests/fixtures/site
#   4) 以验证码回显方式启动 API：
#      $env:JOBSROBOT_AUTH_EXPOSE_CODE_IN_RESPONSE='true'; go run ./cmd/api
#
# 用法：powershell -File tests/smoke.ps1
#
# ℹ️ 关于 ID：所有实体标识都是 **ULID 字符串**（26 位，如 01JD7Q9K2M3N4P5R6S7T8V9W0X），
#    不是数字自增 id。本脚本一律把 ID 当作**不透明字符串**透传（从响应里取出、拼进 URL），
#    不做 [int] 转换、不与数字字面量比较，因此无需随主键形态变化而调整。
#    注意：路径参数必须是合法 ULID，否则接口返回 400（code=10001）。
#
# ⚠️ 两个使用前提：
#
# 1) 本脚本会**临时修改全局配置**并在结束时恢复：邮件配置（console→smtp→console）、
#    抓取收集范围、以及管理员密码。请不要与手工测试同时运行，否则会互相干扰
#    （例如手工注册时正好撞上脚本把邮件驱动切成不可达的 SMTP）。
#
# 2) 认证接口按 IP 限流，脚本自身与手工测试共用同一额度。若出现 429 取不到验证码，
#    可提高额度后重启 API（推荐做法），或直接清空限流键：
#      $env:JOBSROBOT_AUTH_RL_SEND_CODE_LIMIT='500'   # 其余 *_LIMIT 同理
#      docker exec jobs-robot-redis redis-cli --scan --pattern 'jobsrobot:ratelimit:*' |
#        ForEach-Object { docker exec jobs-robot-redis redis-cli DEL $_ }

param(
    [string]$Base = 'http://127.0.0.1:8090',
    [string]$Site = 'http://127.0.0.1:8899',
    [string]$AdminEmail = 'admin@example.com',
    [string]$AdminPassword = 'AdminPassw0rd',
    # 是否执行「会改动全局邮件配置」的用例。
    #
    # 默认关闭：SMTP 密码在接口上只以占位符返回，脚本无法备份/还原它，
    # 一旦写入测试用的 SMTP 配置就会破坏管理员已配置的真实凭据。
    # 独立环境（CI）可用 -MailMutation 开启完整覆盖。
    [switch]$MailMutation
)

$ErrorActionPreference = 'Stop'
$script:Pass = 0
$script:Fail = 0
# Skip 用于「因环境数据不同而无法构造的场景」，必须显式报出而不是静默略过。
$script:Skip = 0
$script:RunId = (Get-Date -Format 'MMddHHmmss')
$script:NewUserEmail = "smoke-$script:RunId@example.com"
$script:ReviewerEmail = "reviewer-$script:RunId@example.com"
$script:CompanyName = "Smoke Example Tech $script:RunId"
# 临时管理员密码按次生成，避免出现「新旧密码相同」导致改密被拒（ErrPasswordSame），
# 也保证脚本结束时能准确还原为 -AdminPassword 指定的密码。
$script:TempAdminPassword = "NewAdmin-$script:RunId-Passw0rd"

function Check {
    param([string]$Name, [bool]$Condition, [string]$Detail = '')
    if ($Condition) {
        $script:Pass++
        Write-Host "  [PASS] $Name" -ForegroundColor Green
    } else {
        $script:Fail++
        Write-Host "  [FAIL] $Name  $Detail" -ForegroundColor Red
    }
}

function Section {
    param([string]$Title)
    Write-Host ''
    Write-Host "== $Title" -ForegroundColor Cyan
}

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        $Body,
        [string]$Token
    )

    $headers = @{ 'Accept' = 'application/json' }
    if ($Token) { $headers['Authorization'] = "Bearer $Token" }

    $reqArgs = @{
        Method          = $Method
        Uri             = "$Base$Path"
        Headers         = $headers
        UseBasicParsing = $true
        TimeoutSec      = 120
    }

    $tmp = $null
    if ($null -ne $Body) {
        $tmp = Join-Path $env:TEMP ("jobsrobot-body-" + [guid]::NewGuid().ToString('N') + '.json')
        $json = $Body | ConvertTo-Json -Depth 12 -Compress
        [System.IO.File]::WriteAllText($tmp, $json, (New-Object System.Text.UTF8Encoding($false)))
        $reqArgs['InFile'] = $tmp
        $reqArgs['ContentType'] = 'application/json; charset=utf-8'
    }

    try {
        $resp = Invoke-WebRequest @reqArgs
        if ($tmp) { Remove-Item $tmp -ErrorAction SilentlyContinue }
        return [pscustomobject]@{ Status = [int]$resp.StatusCode; Json = ($resp.Content | ConvertFrom-Json); Raw = $resp.Content }
    } catch {
        if ($tmp) { Remove-Item $tmp -ErrorAction SilentlyContinue }

        $webResp = $_.Exception.Response
        if ($null -eq $webResp) { throw }

        $status = 0
        try { $status = [int]$webResp.StatusCode } catch { }

        # PowerShell 7 使用 HttpClient，错误响应体通过 ErrorDetails 或 Content 读取。
        $text = ''
        if ($_.ErrorDetails -and $_.ErrorDetails.Message) {
            $text = $_.ErrorDetails.Message
        } else {
            try { $text = $webResp.Content.ReadAsStringAsync().GetAwaiter().GetResult() } catch { }
        }

        $parsed = $null
        try { $parsed = $text | ConvertFrom-Json } catch { }
        return [pscustomobject]@{ Status = $status; Json = $parsed; Raw = $text }
    }
}

Write-Host "jobs-robot 冒烟测试  Base=$Base  Site=$Site" -ForegroundColor Yellow

# ---------------------------------------------------------------- 前置条件
Section '前置条件检查'

# 脚本需要从接口直接拿到验证码，否则注册/找回密码/审核员创建等用例会大面积失败。
# 这里先探测一次，把「配置没开」这类环境问题变成一条可执行的提示，
# 而不是让人对着几十条级联失败去猜原因。
$probeEmail = "probe-$script:RunId-$((Get-Random -Maximum 99999))@example.com"
$probe = Invoke-Api POST '/api/v1/auth/codes' @{ email = $probeEmail; purpose = 'register' }
if ($probe.Status -ne 200 -or -not $probe.Json.data.debug_code) {
    Write-Host ''
    Write-Host "前置条件不满足：接口未回显验证码（status=$($probe.Status) code=$($probe.Json.code)）。" -ForegroundColor Red
    Write-Host '请以联调模式启动 API 后重试：' -ForegroundColor Yellow
    Write-Host "  `$env:JOBSROBOT_AUTH_EXPOSE_CODE_IN_RESPONSE='true'; go run ./cmd/api" -ForegroundColor Yellow
    if ($probe.Status -eq 429) {
        Write-Host '当前返回 429：认证接口的 IP 限流额度已用尽，可提高额度后重启：' -ForegroundColor Yellow
        Write-Host "  `$env:JOBSROBOT_AUTH_RL_SEND_CODE_LIMIT='500'" -ForegroundColor Yellow
    }
    exit 1
}
Check '前置条件：验证码可回显（联调模式已开启）' $true

# 任何非 HTTP 层的意外异常都要打印汇总，避免"静默失败"。
trap {
    Write-Host ''
    Write-Host "脚本异常终止：$($_.Exception.Message)" -ForegroundColor Red
    Write-Host "通过：$script:Pass  失败：$script:Fail" -ForegroundColor Red
    exit 1
}

# ---------------------------------------------------------------- 未鉴权
Section '未鉴权访问拦截'

$r = Invoke-Api GET '/api/v1/admin/stats'
Check '未登录访问管理接口返回 401' ($r.Status -eq 401) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/me'
Check '未登录访问 /me 返回 401' ($r.Status -eq 401) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/jobs'
Check '未登录可浏览公开岗位列表' ($r.Status -eq 200) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/does-not-exist'
Check '未知路由返回统一信封的错误码' ($r.Status -eq 404 -and $r.Json.code -eq 10004) "status=$($r.Status) code=$($r.Json.code)"

# ---------------------------------------------------------------- 认证
Section '登录与令牌'

$r = Invoke-Api POST '/api/v1/auth/login' @{ email = $AdminEmail; password = $AdminPassword }
Check '管理员登录成功' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"if ($r.Status -ne 200) {
    # 快速失败：管理员登录不通时后续上百项检查都会级联失败，没有参考价值。
    Write-Host ''
    Write-Host '管理员登录失败，后续检查已跳过。请确认 -AdminPassword 与库中密码一致，' -ForegroundColor Red
    Write-Host '或以验证码方式重置管理员密码后再运行：' -ForegroundColor Red
    Write-Host '  POST /api/v1/auth/password/forgot {"email":"admin@example.com"}' -ForegroundColor Yellow
    Write-Host '  POST /api/v1/auth/password/reset  {"email":"admin@example.com","code":"<验证码>","new_password":"AdminPassw0rd"}' -ForegroundColor Yellow
    exit 1
}
$adminToken = $r.Json.data.token.access_token
$adminRefresh = $r.Json.data.token.refresh_token
Check '登录返回访问令牌与刷新令牌' ($adminToken -and $adminRefresh)
Check '登录响应包含用户角色' ($r.Json.data.user.roles -contains 'admin') "roles=$($r.Json.data.user.roles -join ',')"
# 记录当前管理员 ID：后续「自我约束」检查基于它，不硬编码 /users/1，
# 避免依赖数据库自增顺序这一环境假设。
$adminUserId = $r.Json.data.user.id

$r = Invoke-Api GET '/api/v1/me' -Token $adminToken
Check '携带令牌访问 /me 成功' ($r.Status -eq 200 -and $r.Json.data.email -eq $AdminEmail) "status=$($r.Status)"

$r = Invoke-Api POST '/api/v1/auth/login' @{ email = $AdminEmail; password = 'WrongPassw0rd' }
Check '错误密码登录返回 401' ($r.Status -eq 401 -and $r.Json.code -eq 20002) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST '/api/v1/auth/refresh' @{ refresh_token = $adminRefresh }
Check '刷新令牌成功' ($r.Status -eq 200 -and $r.Json.data.access_token) "status=$($r.Status)"
$adminToken = $r.Json.data.access_token

$r = Invoke-Api POST '/api/v1/auth/refresh' @{ refresh_token = 'not-a-real-token' }
Check '伪造刷新令牌被拒绝' ($r.Status -eq 401) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/me/sessions' -Token $adminToken
Check '会话列表可查询' ($r.Status -eq 200 -and $r.Json.data.items.Count -ge 1) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/admin/stats' -Token $adminToken
Check '管理员可访问统计接口' ($r.Status -eq 200 -and $r.Json.data.users.total -ge 1) "status=$($r.Status) body=$($r.Raw)"

$r = Invoke-Api GET '/api/v1/meta/dictionaries' -Token $adminToken
Check '字典接口返回岗位状态枚举' ($r.Status -eq 200 -and $r.Json.data.job_statuses.Count -eq 5) "status=$($r.Status)"

# ---------------------------------------------------------------- 注册 / 找回密码
Section '邮箱注册与找回密码'

$r = Invoke-Api POST '/api/v1/auth/codes' @{ email = $script:NewUserEmail; purpose = 'register' }
Check '注册验证码发送成功' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
$regCode = $r.Json.data.debug_code
Check '联调模式返回验证码' ([bool]$regCode)

$r = Invoke-Api POST '/api/v1/auth/register' @{ email = $script:NewUserEmail; password = 'UserPassw0rd'; code = '000000' }
Check '错误验证码无法注册' ($r.Status -eq 400 -and $r.Json.code -eq 20005) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST '/api/v1/auth/register' @{ email = $script:NewUserEmail; password = 'UserPassw0rd'; nickname = 'Smoke User'; code = $regCode }
Check '正确验证码注册成功' ($r.Status -eq 201) "status=$($r.Status) body=$($r.Raw)"
$userToken = $r.Json.data.token.access_token
$newUserId = $r.Json.data.user.id
Check '新用户默认获得 user 角色' ($r.Json.data.user.roles -contains 'user') "roles=$($r.Json.data.user.roles -join ',')"

$r = Invoke-Api POST '/api/v1/auth/register' @{ email = $script:NewUserEmail; password = 'UserPassw0rd'; code = $regCode }
Check '重复邮箱注册被拒绝' ($r.Status -eq 409 -and $r.Json.code -eq 20001) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST '/api/v1/auth/register' @{ email = "weak-$script:RunId@example.com"; password = 'short' }
Check '弱密码注册被拒绝' ($r.Status -eq 400) "status=$($r.Status)"

$r = Invoke-Api POST '/api/v1/auth/password/forgot' @{ email = $script:NewUserEmail }
Check '找回密码验证码发送成功' ($r.Status -eq 200 -and $r.Json.data.debug_code) "status=$($r.Status)"
$forgotCode = $r.Json.data.debug_code

$r = Invoke-Api POST '/api/v1/auth/password/forgot' @{ email = "ghost-$script:RunId@example.com" }
Check '未注册邮箱不泄露账号是否存在' ($r.Status -eq 200 -and -not $r.Json.data.debug_code) "status=$($r.Status) body=$($r.Raw)"

# 同一邮箱存在发送冷却：这里复用上一步拿到的验证码，并顺带验证冷却确实生效。
Check '重置密码验证码已获取' ([bool]$forgotCode)

$r = Invoke-Api POST '/api/v1/auth/codes' @{ email = $script:NewUserEmail; purpose = 'reset_password' }
Check '冷却期内重复索取验证码被限流（429）' ($r.Status -eq 429 -and $r.Json.code -eq 20006) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST '/api/v1/auth/password/reset' @{ email = $script:NewUserEmail; code = $forgotCode; new_password = 'ResetPassw0rd' }
Check '重置密码成功' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"

$r = Invoke-Api POST '/api/v1/auth/login' @{ email = $script:NewUserEmail; password = 'ResetPassw0rd' }
Check '新密码可登录' ($r.Status -eq 200) "status=$($r.Status)"
# 重置密码会吊销全部会话，因此必须重新登录后再验证越权拦截。
$userToken = $r.Json.data.token.access_token

$r = Invoke-Api POST '/api/v1/auth/login' @{ email = $script:NewUserEmail; password = 'UserPassw0rd' }
Check '旧密码已失效' ($r.Status -eq 401) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/admin/stats' -Token $userToken
Check '普通用户访问管理接口返回 403' ($r.Status -eq 403) "status=$($r.Status)"

# ---------------------------------------------------------------- 公司 + 抓取
Section '公司录入与招聘入口发现'

# 夹具岗位的 source_url 是固定的。若上一次运行异常退出（例如令牌失效导致清理未执行），
# 库里会残留同 URL 的岗位，本轮抓取就会表现为「已存在」而不是「新建」，
# 把残留数据误判成抓取缺陷。这里按来源地址先清一遍，保证脚本可重复运行。
$fixtureURLs = @(
    "$Site/jobs/backend.html",
    "$Site/jobs/product.html",
    "$Site/jobs/outsourcing.html"
)
$preCleaned = 0
foreach ($fixtureURL in $fixtureURLs) {
    $encoded = [uri]::EscapeDataString($fixtureURL)
    $found = Invoke-Api GET "/api/v1/admin/jobs?source_url=$encoded&page_size=50" -Token $adminToken
    foreach ($item in @($found.Json.data.items)) {
        $d = Invoke-Api DELETE "/api/v1/admin/jobs/$($item.id)" -Token $adminToken
        if ($d.Status -eq 200) { $preCleaned++ }
    }
}
if ($preCleaned -gt 0) {
    Write-Host "  [INFO] 已清理上一轮残留的夹具岗位 $preCleaned 个（按 source_url 精确匹配）" -ForegroundColor DarkGray
}
$r = Invoke-Api GET "/api/v1/admin/jobs?source_url=$([uri]::EscapeDataString($fixtureURLs[0]))&page_size=50" -Token $adminToken
Check '可按来源地址精确检索岗位' ($r.Status -eq 200 -and @($r.Json.data.items).Count -eq 0) "status=$($r.Status) count=$(@($r.Json.data.items).Count)"

# 数据库中的 crawler.scope 会覆盖静态配置（这正是「热更新」的设计），
# 因此本用例会临时把范围改成与夹具匹配的值——但必须先备份原值并在收尾时还原，
# 否则会把管理员此前配置的收集范围冲掉。
$scopeBackupResponse = Invoke-Api GET '/api/v1/admin/crawl-scope' -Token $adminToken
$scopeBackup = $scopeBackupResponse.Json.data
Check '可读取当前收集范围用于备份' ($scopeBackupResponse.Status -eq 200) "status=$($scopeBackupResponse.Status)"

$r = Invoke-Api PUT '/api/v1/admin/crawl-scope' -Token $adminToken -Body @{
    job_types         = @('技术', '产品', '设计', '运营', '市场', '职能')
    industries        = @('互联网', '人工智能', '金融科技', '智能制造', '新能源')
    employment_types  = @('全职', '实习', '校招', '社招')
    exclude_keywords  = @('外包', '劳务派遣', '中介')
    min_quality_score = 0
}
Check '抓取前重置收集范围' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"

try {
    $siteResp = Invoke-WebRequest -Uri "$Site/robots.txt" -UseBasicParsing -TimeoutSec 10
    Check '模拟企业官网 robots.txt 可访问' ([int]$siteResp.StatusCode -eq 200) "status=$($siteResp.StatusCode)"
} catch {
    Check '模拟企业官网 robots.txt 可访问' $false "无法访问 $Site/robots.txt（请先启动模拟官网）"
}

$r = Invoke-Api POST '/api/v1/admin/companies' -Token $adminToken -Body @{
    name        = $script:CompanyName
    website     = $Site
    industry    = '智能制造'
    location    = 'Beijing'
    description = 'Smoke test company'
} 
Check '新增公司成功' ($r.Status -eq 201) "status=$($r.Status) body=$($r.Raw)"
$companyId = $r.Json.data.id
Check '新增后 careers_url 为空，等待发现流程回填' (-not $r.Json.data.careers_url) "careers_url=$($r.Json.data.careers_url)"

$r = Invoke-Api POST '/api/v1/admin/companies' -Token $adminToken -Body @{ name = $script:CompanyName; website = $Site }
Check '同名公司重复创建被拒绝' ($r.Status -eq 409) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST '/api/v1/admin/companies' -Token $adminToken -Body @{ name = "Bad URL Co $script:RunId"; website = 'not-a-url' }
Check '非法官网地址被拒绝' ($r.Status -eq 400) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST "/api/v1/admin/companies/$companyId/discover" -Token $adminToken
Check '自动发现《加入我们》入口成功' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
Check '入口指向 join-us 页面' ($r.Json.data.careers_url -eq "$Site/join-us.html") "careers_url=$($r.Json.data.careers_url)"
Check '记录了发现来源页面' ([bool]$r.Json.data.discovery_url) "discovery_url=$($r.Json.data.discovery_url)"

Section '真实抓取与岗位落库'

$r = Invoke-Api POST "/api/v1/admin/companies/$companyId/crawl" -Token $adminToken
Check '触发抓取成功' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
$run = $r.Json.data
Check '抓取运行状态为 success' ($run.status -eq 'success') "status=$($run.status) error=$($run.error_message)"
Check '发现 3 个岗位链接' ($run.jobs_found -eq 3) "jobs_found=$($run.jobs_found)"
Check '入库 2 个岗位' ($run.jobs_created -eq 2) "jobs_created=$($run.jobs_created)"
Check '过滤掉 1 个岗位（外包关键词）' ($run.jobs_filtered -ge 1) "jobs_filtered=$($run.jobs_filtered)"
Check '抓取页数大于 0' ($run.pages_fetched -gt 0) "pages_fetched=$($run.pages_fetched)"
$runId = $run.id

$r = Invoke-Api GET '/api/v1/admin/jobs?status=pending_review&page_size=50' -Token $adminToken
Check '待审核岗位列表可查询' ($r.Status -eq 200) "status=$($r.Status)"
$pending = @($r.Json.data.items | Where-Object { $_.company.id -eq $companyId })
Check '新采集岗位进入待审核' ($pending.Count -eq 2) "count=$($pending.Count)"

$backendJob = $pending | Where-Object { $_.title -like '*Go*' } | Select-Object -First 1
Check '抓取到 Go 后端岗位' ([bool]$backendJob) "titles=$($pending.title -join ' | ')"

$r = Invoke-Api GET "/api/v1/admin/jobs/$($backendJob.id)" -Token $adminToken
$job = $r.Json.data
Check '岗位详情可读取' ($r.Status -eq 200) "status=$($r.Status)"
Check '抽取到任职要求' ($job.requirements -like '*PostgreSQL*' -and $job.requirements -like '*Kubernetes*') "requirements=$($job.requirements)"
Check '任职要求已结构化为条目' ($job.requirements_list.Count -ge 3) "list=$($job.requirements_list.Count)"
Check '抽取到岗位职责' ($job.responsibilities -like '*设备接入*') "responsibilities=$($job.responsibilities)"
Check '抽取到联系方式邮箱' ($job.contact_email -eq 'hr@example-tech.com') "email=$($job.contact_email)"
Check '抽取到联系电话' ($job.contact_phone -like '*88886666*') "phone=$($job.contact_phone)"
Check '抽取到工作地址' ($job.address -like '*SOHO*') "address=$($job.address)"
Check '抽取到城市' ($job.city -like '*北京*') "city=$($job.city)"
Check '抽取到发布时间' ($job.published_at -like '2025-01-08*') "published_at=$($job.published_at)"
Check '抽取到薪资范围' ($job.salary_min -eq 30000 -and $job.salary_max -eq 50000) "min=$($job.salary_min) max=$($job.salary_max)"
Check '判定为官网直招' ($job.is_direct_hire -eq $true)
Check '记录来源地址与站点' ($job.source_url -like '*backend.html' -and $job.source_site -like '*8899*') "url=$($job.source_url)"
Check '记录最后更新时间' ([bool]$job.last_updated_at -and [bool]$job.last_seen_at)
Check 'JSON-LD 抽取器生效' ($job.extractor -eq 'jsonld') "extractor=$($job.extractor)"
Check '质量评分不为零' ($job.quality_score -gt 0) "score=$($job.quality_score)"
Check '岗位状态为待审核' ($job.status -eq 'pending_review') "status=$($job.status)"

$productJob = $pending | Where-Object { $_.title -like '*产品*' } | Select-Object -First 1
Check '抓取到纯 DOM 页面的岗位' ([bool]$productJob)
if ($productJob) {
    $r = Invoke-Api GET "/api/v1/admin/jobs/$($productJob.id)" -Token $adminToken
    $pjob = $r.Json.data
    Check 'DOM 启发式抽取到任职要求' ($pjob.requirements -like '*B*产品*' -or $pjob.requirements -like '*产品经验*') "requirements=$($pjob.requirements)"
    Check 'DOM 抽取器标记正确' ($pjob.extractor -in @('dom', 'rules')) "extractor=$($pjob.extractor)"
    Check '抽取到福利待遇' ($pjob.benefits -like '*六险一金*') "benefits=$($pjob.benefits)"
    Check '抽取到招聘人数' ($pjob.headcount -eq 1) "headcount=$($pjob.headcount)"
}

$r = Invoke-Api GET "/api/v1/admin/jobs/$($backendJob.id)/revisions" -Token $adminToken
Check '生成初始修订记录' ($r.Status -eq 200 -and $r.Json.data.items.Count -ge 1) "count=$($r.Json.data.items.Count)"

$r = Invoke-Api GET "/api/v1/admin/crawl-runs/$runId" -Token $adminToken
Check '抓取明细日志可查询' ($r.Status -eq 200 -and $r.Json.data.logs.Count -gt 0) "logs=$($r.Json.data.logs.Count)"
$filterLog = $r.Json.data.logs | Where-Object { $_.stage -eq 'filter' } | Select-Object -First 1
Check '过滤原因被记录到日志' ([bool]$filterLog) "messages=$($r.Json.data.logs.message -join ' | ')"
if ($filterLog) {
    Check '过滤原因说明命中了排除关键词' ($filterLog.message -like '*关键词*') "message=$($filterLog.message)"
}
$discoverLog = $r.Json.data.logs | Where-Object { $_.stage -eq 'discover' } | Select-Object -First 1
Check '记录招聘入口发现过程' ([bool]$discoverLog) "stages=$($r.Json.data.logs.stage -join ',')"

Section '幂等重抓'

$r = Invoke-Api POST "/api/v1/admin/companies/$companyId/crawl" -Token $adminToken
$rerun = $r.Json.data
Check '二次抓取仍成功' ($rerun.status -eq 'success') "status=$($rerun.status)"
Check '二次抓取不重复入库' ($rerun.jobs_created -eq 0) "created=$($rerun.jobs_created)"
Check '内容未变化被识别为 unchanged' ($rerun.jobs_unchanged -eq 2) "unchanged=$($rerun.jobs_unchanged) updated=$($rerun.jobs_updated)"
Check '二次抓取未新增修订记录' ($rerun.jobs_updated -eq 0) "updated=$($rerun.jobs_updated)"

$r = Invoke-Api GET "/api/v1/admin/jobs/$($backendJob.id)/revisions" -Token $adminToken
Check '修订记录数量未增长' ($r.Json.data.items.Count -eq 1) "count=$($r.Json.data.items.Count)"

Section '审核与发布'

$r = Invoke-Api POST "/api/v1/admin/jobs/$($productJob.id)/review" -Token $adminToken -Body @{ decision = 'reject'; reject_reason = 'Smoke test rejection' }
Check '审核驳回成功' ($r.Status -eq 200 -and $r.Json.data.status -eq 'rejected') "status=$($r.Status) job=$($r.Json.data.status)"

$r = Invoke-Api POST "/api/v1/admin/jobs/$($backendJob.id)/review" -Token $adminToken -Body @{ decision = 'approve' }
Check '审核通过成功' ($r.Status -eq 200 -and $r.Json.data.status -eq 'approved') "status=$($r.Status) job=$($r.Json.data.status)"
Check '记录审核人' ([bool]$r.Json.data.reviewer_id) "reviewer_id=$($r.Json.data.reviewer_id)"

$r = Invoke-Api POST "/api/v1/admin/jobs/$($backendJob.id)/review" -Token $adminToken -Body @{ decision = 'approve' }
Check '重复审核已发布岗位被拒绝' ($r.Status -eq 409) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST "/api/v1/admin/jobs/$($productJob.id)/review" -Token $adminToken -Body @{ decision = 'reject' }
Check '驳回必须填写原因' ($r.Status -eq 400) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/jobs?page_size=50'
$publicJobs = @(@($r.Json.data.items) | Where-Object { $_.company.id -eq $companyId })
Check '公开列表只包含已发布岗位' ($publicJobs.Count -eq 1) "count=$($publicJobs.Count)"

$r = Invoke-Api GET "/api/v1/jobs/$($backendJob.id)"
Check '公开详情可访问已发布岗位' ($r.Status -eq 200) "status=$($r.Status)"
Check '公开详情包含任职要求' ($r.Json.data.requirements -like '*PostgreSQL*') "requirements=$($r.Json.data.requirements)"
Check '公开详情包含联系方式' ([bool]$r.Json.data.contact_email) "email=$($r.Json.data.contact_email)"

$r = Invoke-Api GET "/api/v1/jobs/$($productJob.id)"
Check '未发布岗位的公开详情不可见' ($r.Status -eq 404) "status=$($r.Status)"

Section '筛选与相关职位'

# 夹具岗位薪资为 30000-50000，经验要求「3-5 年」，用于验证区间相交与包含匹配语义。
$r = Invoke-Api GET '/api/v1/jobs?salary_min=40000&page_size=50'
$hit = @(@($r.Json.data.items) | Where-Object { $_.id -eq $backendJob.id })
Check '薪资下限与岗位区间相交时命中' ($hit.Count -eq 1) "count=$($hit.Count)"

$r = Invoke-Api GET '/api/v1/jobs?salary_min=60000&page_size=50'
$miss = @(@($r.Json.data.items) | Where-Object { $_.id -eq $backendJob.id })
Check '薪资下限高于岗位上限时不命中' ($miss.Count -eq 0) "count=$($miss.Count)"

$r = Invoke-Api GET '/api/v1/jobs?salary_max=20000&page_size=50'
$miss = @(@($r.Json.data.items) | Where-Object { $_.id -eq $backendJob.id })
Check '薪资上限低于岗位下限时不命中' ($miss.Count -eq 0) "count=$($miss.Count)"

$r = Invoke-Api GET '/api/v1/jobs?experience=3-5&page_size=50'
$hit = @(@($r.Json.data.items) | Where-Object { $_.id -eq $backendJob.id })
Check '经验要求筛选命中' ($hit.Count -eq 1) "count=$($hit.Count)"

$r = Invoke-Api GET '/api/v1/jobs?experience=10年以上&page_size=50'
$miss = @(@($r.Json.data.items) | Where-Object { $_.id -eq $backendJob.id })
Check '经验要求不匹配时不命中' ($miss.Count -eq 0) "count=$($miss.Count)"

$r = Invoke-Api GET "/api/v1/jobs/$($backendJob.id)/related"
Check '相关职位接口可用' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
Check '相关职位返回列表结构' ($null -ne $r.Json.data.items) "body=$($r.Raw)"

$r = Invoke-Api POST "/api/v1/admin/jobs/$($backendJob.id)/status" -Token $adminToken -Body @{ status = 'offline' }
Check '岗位可下线' ($r.Status -eq 200 -and $r.Json.data.status -eq 'offline') "status=$($r.Json.data.status)"

$r = Invoke-Api GET "/api/v1/jobs/$($backendJob.id)"
Check '下线后公开详情不可见' ($r.Status -eq 404) "status=$($r.Status)"

$r = Invoke-Api POST "/api/v1/admin/jobs/$($backendJob.id)/status" -Token $adminToken -Body @{ status = 'approved' }
Check '岗位可重新上线' ($r.Status -eq 200 -and $r.Json.data.status -eq 'approved') "status=$($r.Json.data.status)"

Section '人工编辑与变更追溯'

$r = Invoke-Api PATCH "/api/v1/admin/jobs/$($backendJob.id)" -Token $adminToken -Body @{ salary_text = '35-55K x14'; city = 'Beijing' }
Check '人工编辑岗位成功' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
Check '编辑后内容更新' ($r.Json.data.salary_text -eq '35-55K x14') "salary_text=$($r.Json.data.salary_text)"

$r = Invoke-Api GET "/api/v1/admin/jobs/$($backendJob.id)/revisions" -Token $adminToken
Check '人工编辑生成新修订记录' ($r.Json.data.items.Count -eq 2) "count=$($r.Json.data.items.Count)"
$manualRev = $r.Json.data.items | Where-Object { $_.trigger -eq 'manual' } | Select-Object -First 1
Check '修订记录标记为人工编辑' ([bool]$manualRev) "triggers=$($r.Json.data.items.trigger -join ',')"
if ($manualRev) {
    Check '修订记录列出变更字段' ($manualRev.changed_fields -contains 'salary_text') "fields=$($manualRev.changed_fields -join ',')"
}
Check '实质性变更使已发布岗位退回待审核' ((Invoke-Api GET "/api/v1/admin/jobs/$($backendJob.id)" -Token $adminToken).Json.data.status -eq 'pending_review')

# ---------------------------------------------------------------- RBAC
Section 'RBAC 三角色越权拦截'

$r = Invoke-Api POST '/api/v1/auth/codes' @{ email = $script:ReviewerEmail; purpose = 'register' }
$reviewerCode = $r.Json.data.debug_code
Check '审核员账号验证码已获取' ([bool]$reviewerCode)

$r = Invoke-Api POST '/api/v1/auth/register' @{ email = $script:ReviewerEmail; password = 'ReviewerPassw0rd'; code = $reviewerCode }
Check '审核员账号注册成功' ($r.Status -eq 201) "status=$($r.Status)"
$reviewerId = $r.Json.data.user.id

$r = Invoke-Api POST "/api/v1/admin/users/$reviewerId/roles" -Token $adminToken -Body @{ roles = @('reviewer') }
Check '管理员可授予审核员角色' ($r.Status -eq 200 -and ($r.Json.data.roles -contains 'reviewer')) "status=$($r.Status) roles=$($r.Json.data.roles -join ',')"

$r = Invoke-Api POST '/api/v1/auth/login' @{ email = $script:ReviewerEmail; password = 'ReviewerPassw0rd' }
Check '审核员登录成功（角色变更后旧令牌失效，需重新登录）' ($r.Status -eq 200) "status=$($r.Status)"
$reviewerToken = $r.Json.data.token.access_token

$r = Invoke-Api GET '/api/v1/admin/jobs?status=pending_review' -Token $reviewerToken
Check '审核员可查看待审核列表' ($r.Status -eq 200) "status=$($r.Status)"

# 用「被管理员驳回的岗位」验证审核员权限：驳回态允许改判。
$r = Invoke-Api POST "/api/v1/admin/jobs/$($productJob.id)/review" -Token $reviewerToken -Body @{ decision = 'approve' }
Check '审核员可执行审核' ($r.Status -eq 200 -and $r.Json.data.status -eq 'approved') "status=$($r.Status) body=$($r.Raw)"

$r = Invoke-Api GET '/api/v1/admin/companies' -Token $reviewerToken
Check '审核员可只读查看公司列表' ($r.Status -eq 200) "status=$($r.Status)"

$r = Invoke-Api POST '/api/v1/admin/companies' -Token $reviewerToken -Body @{ name = "Forbidden Co $script:RunId" }
Check '审核员不能新增公司（403）' ($r.Status -eq 403 -and $r.Json.code -eq 10003) "status=$($r.Status) code=$($r.Json.code)"

# 这里只需要一个合法 ULID 形态的路径参数：鉴权（403）在 handler 之前就返回，
# 不依赖该用户是否真实存在。
$r = Invoke-Api DELETE '/api/v1/admin/users/01JD7Q9K2M3N4P5R6S7T8V9W0X' -Token $reviewerToken
Check '审核员不能删除用户（403）' ($r.Status -eq 403) "status=$($r.Status)"

$r = Invoke-Api PUT '/api/v1/admin/crawl-scope' -Token $reviewerToken -Body @{ job_types = @('Engineering') }
Check '审核员不能修改收集范围（403）' ($r.Status -eq 403) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/admin/audit-logs' -Token $reviewerToken
Check '审核员不能查看审计日志（403）' ($r.Status -eq 403) "status=$($r.Status)"

# ---------------------------------------------------------------- 配置热更新
Section '收集范围热更新'

$r = Invoke-Api PUT '/api/v1/admin/crawl-scope' -Token $adminToken -Body @{
    job_types         = @('技术')
    industries        = @('智能制造')
    employment_types  = @('全职')
    exclude_keywords  = @('外包', '劳务派遣')
    min_quality_score = 30
}
Check '保存收集范围成功' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"

$r = Invoke-Api GET '/api/v1/admin/crawl-scope' -Token $adminToken
Check '收集范围可回读' ($r.Status -eq 200 -and $r.Json.data.job_types -contains '技术') "job_types=$($r.Json.data.job_types -join ',')"
Check '排除关键词已保存' ($r.Json.data.exclude_keywords -contains '外包') "exclude=$($r.Json.data.exclude_keywords -join ',')"
Check '质量分阈值已保存' ($r.Json.data.min_quality_score -eq 30) "score=$($r.Json.data.min_quality_score)"

$r = Invoke-Api POST "/api/v1/admin/companies/$companyId/crawl" -Token $adminToken
Check '新范围立即生效（本轮不再入库）' ($r.Json.data.jobs_created -eq 0) "created=$($r.Json.data.jobs_created)"

# ---------------------------------------------------------------- 邮件设置
Section '邮件设置在后台在线生效'

$r = Invoke-Api GET '/api/v1/admin/mail' -Token $adminToken
Check '管理员可读取邮件配置' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
Check '邮件配置来源已标记' ($r.Json.data.source -in @('config', 'database')) "source=$($r.Json.data.source)"
Check '配置驱动与生效驱动一致' ($r.Json.data.active_driver -eq $r.Json.data.driver) "active=$($r.Json.data.active_driver) driver=$($r.Json.data.driver)"
Check '响应以占位符代替密码' ($r.Json.data.password_mask -eq '********') "mask=$($r.Json.data.password_mask)"

$r = Invoke-Api GET '/api/v1/admin/mail' -Token $reviewerToken
Check '审核员无权读取邮件配置（403）' ($r.Status -eq 403) "status=$($r.Status)"

# —— 以下为「请求会被拒绝、不会落库」的校验用例，始终执行 ——
$r = Invoke-Api PUT '/api/v1/admin/mail' -Token $adminToken -Body @{ driver = 'smtp'; host = ''; port = 0; from_address = '' }
Check '不完整的 SMTP 配置被拒绝' ($r.Status -eq 400) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api PUT '/api/v1/admin/mail' -Token $adminToken -Body @{ driver = 'carrier-pigeon' }
Check '不支持的驱动被拒绝' ($r.Status -eq 400) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api PUT '/api/v1/admin/mail' -Token $adminToken -Body @{
    driver = 'smtp'; host = 'smtp.example.com'; port = 465; from_address = 'no-reply@example.com'; username = 'only-user'
}
Check '用户名与密码必须同时填写' ($r.Status -eq 400) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST '/api/v1/admin/mail/test' -Token $adminToken -Body @{ to = 'not-an-email' }
Check '测试发信校验收件邮箱' ($r.Status -eq 400) "status=$($r.Status)"

# —— 会改动全局邮件配置的用例 ——
#
# 默认跳过：SMTP 密码在接口上只以占位符返回，脚本无法备份并还原它，
# 一旦写入测试用的 SMTP 配置，就会破坏管理员已配置的真实凭据。
if (-not $MailMutation) {
    $script:Skip += 14
    Write-Host '  [SKIP] 邮件配置的保存 / 热生效 / 测试发信 / 回落 共 14 项' -ForegroundColor Yellow
    Write-Host '         原因：默认不修改全局邮件配置（SMTP 密码无法备份还原）' -ForegroundColor DarkGray
    Write-Host '         独立环境（CI）请加 -MailMutation 以覆盖这些用例' -ForegroundColor DarkGray
} else {
    # 故意指向一个没有服务监听的端口：保存应当成功（保存配置不应要求连通性），
    # 但发测试邮件必须失败并给出可读原因。
    $secretPassword = 'SmtpSecret-12345'
    $r = Invoke-Api PUT '/api/v1/admin/mail' -Token $adminToken -Body @{
        driver       = 'smtp'
        from_name    = 'JobHub 直招'
        from_address = 'no-reply@example.com'
        host         = '127.0.0.1'
        port         = 2525
        username     = 'smtp-user'
        password     = $secretPassword
        tls          = $false
    }
    Check '保存 SMTP 配置成功' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
    Check '保存后立即热生效（驱动切到 smtp）' ($r.Json.data.active_driver -eq 'smtp') "active=$($r.Json.data.active_driver)"
    Check '已记录密码已配置' ($r.Json.data.has_password -eq $true) "has_password=$($r.Json.data.has_password)"
    Check '保存响应不回显明文密码' (-not $r.Raw.Contains($secretPassword)) "body=$($r.Raw)"

    $r = Invoke-Api GET '/api/v1/admin/mail' -Token $adminToken
    Check '读取时同样不回显明文密码' (-not $r.Raw.Contains($secretPassword)) "body=$($r.Raw)"
    Check '来源标记为数据库' ($r.Json.data.source -eq 'database') "source=$($r.Json.data.source)"
    Check '记录了修改人' ($r.Json.data.updated_by -eq $AdminEmail) "updated_by=$($r.Json.data.updated_by)"

    $r = Invoke-Api POST '/api/v1/admin/mail/test' -Token $adminToken -Body @{ to = $AdminEmail }
    Check '不可达 SMTP 的测试发信失败并返回 503' ($r.Status -eq 503) "status=$($r.Status) body=$($r.Raw)"

    # 密码留空（不回传占位符）应沿用已保存的密码，而不是被清空。
    $r = Invoke-Api PUT '/api/v1/admin/mail' -Token $adminToken -Body @{
        driver       = 'smtp'
        from_name    = 'JobHub 直招'
        from_address = 'no-reply@example.com'
        host         = '127.0.0.1'
        port         = 2525
        username     = 'smtp-user'
        tls          = $false
    }
    Check '未传密码时沿用已保存密码' ($r.Status -eq 200 -and $r.Json.data.has_password -eq $true) "status=$($r.Status) has_password=$($r.Json.data.has_password)"

    $r = Invoke-Api PUT '/api/v1/admin/mail' -Token $adminToken -Body @{
        driver         = 'smtp'
        from_address   = 'no-reply@example.com'
        host           = '127.0.0.1'
        port           = 2525
        clear_password = $true
    }
    Check '可显式清空密码' ($r.Status -eq 200 -and $r.Json.data.has_password -eq $false) "status=$($r.Status) has_password=$($r.Json.data.has_password)"

    $r = Invoke-Api PUT '/api/v1/admin/mail' -Token $adminToken -Body @{
        driver       = 'console'
        from_name    = 'JobHub 直招'
        from_address = 'no-reply@example.com'
    }
    Check '可切回 console 驱动（本地联调）' ($r.Status -eq 200 -and $r.Json.data.active_driver -eq 'console') "active=$($r.Json.data.active_driver)"

    $r = Invoke-Api POST '/api/v1/admin/mail/test' -Token $adminToken -Body @{ to = $AdminEmail }
    Check 'console 驱动下测试发信成功（内容打印到日志）' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"

    $r = Invoke-Api DELETE '/api/v1/admin/mail' -Token $adminToken
    Check '可回落到配置文件中的邮件配置' ($r.Status -eq 200 -and $r.Json.data.source -eq 'config') "status=$($r.Status) source=$($r.Json.data.source)"

    # 恢复为数据库中的 console 配置，便于脚本重复运行。
    $r = Invoke-Api PUT '/api/v1/admin/mail' -Token $adminToken -Body @{
        driver       = 'console'
        from_name    = 'JobHub 直招'
        from_address = 'no-reply@jobhub.local'
    }
    Check '邮件配置已复位为 console' ($r.Status -eq 200) "status=$($r.Status)"
}

# ---------------------------------------------------------------- 用户与策略
Section '用户、角色与策略'

$r = Invoke-Api GET '/api/v1/admin/users?page_size=50' -Token $adminToken
Check '用户列表可查询' ($r.Status -eq 200 -and $r.Json.data.total -ge 3) "total=$($r.Json.data.total)"

$r = Invoke-Api GET "/api/v1/admin/users?role=reviewer" -Token $adminToken
Check '可按角色筛选用户' ($r.Json.data.total -ge 1) "total=$($r.Json.data.total)"

$r = Invoke-Api GET "/api/v1/admin/users/$reviewerId" -Token $adminToken
Check '用户详情可查询' ($r.Status -eq 200) "status=$($r.Status)"

$r = Invoke-Api POST "/api/v1/admin/users/$reviewerId/status" -Token $adminToken -Body @{ status = 'disabled' }
Check '可禁用用户' ($r.Status -eq 200 -and $r.Json.data.status -eq 'disabled') "status=$($r.Json.data.status)"

$r = Invoke-Api POST '/api/v1/auth/login' @{ email = $script:ReviewerEmail; password = 'ReviewerPassw0rd' }
Check '被禁用账号无法登录（403）' ($r.Status -eq 403 -and $r.Json.code -eq 20003) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api POST "/api/v1/admin/users/$reviewerId/status" -Token $adminToken -Body @{ status = 'active' }
Check '可恢复用户' ($r.Status -eq 200 -and $r.Json.data.status -eq 'active') "status=$($r.Json.data.status)"

# 「最后一名管理员」场景只有在系统确实只剩一名管理员时才能构造。
# 开发库里可能还存在其他管理员（例如用 seed 默认参数建过账号），
# 此时降级是合法的——因此这里先探测再断言，避免把环境差异误报成缺陷。
$r = Invoke-Api GET '/api/v1/admin/users?role=admin&page_size=100' -Token $adminToken
$adminUsers = @($r.Json.data.items)
Check '可按管理员角色筛选用户' ($r.Status -eq 200 -and $adminUsers.Count -ge 1) "status=$($r.Status) count=$($adminUsers.Count)"

if ($adminUsers.Count -eq 1) {
    $r = Invoke-Api POST "/api/v1/admin/users/$($adminUsers[0].id)/roles" -Token $adminToken -Body @{ roles = @('user') }
    Check '最后一名管理员不可被降级' ($r.Status -eq 400) "status=$($r.Status) body=$($r.Raw)"
} else {
    $script:Skip++
    Write-Host "  [SKIP] 最后一名管理员不可被降级 —— 系统中存在 $($adminUsers.Count) 名管理员，无法构造该场景（守卫逻辑由 internal/service 的集成测试覆盖）" -ForegroundColor Yellow
}

$r = Invoke-Api POST "/api/v1/admin/users/$adminUserId/status" -Token $adminToken -Body @{ status = 'disabled' }
Check '不能禁用自己的账号' ($r.Status -eq 400) "status=$($r.Status)"

$r = Invoke-Api DELETE "/api/v1/admin/users/$adminUserId" -Token $adminToken
Check '不能删除自己的账号' ($r.Status -eq 400) "status=$($r.Status)"

$r = Invoke-Api GET '/api/v1/admin/roles' -Token $adminToken
Check '角色列表含三个内置角色' ($r.Json.data.items.Count -eq 3) "count=$($r.Json.data.items.Count)"
$adminRole = $r.Json.data.items | Where-Object { $_.name -eq 'admin' } | Select-Object -First 1
Check '管理员角色带权限策略' ($adminRole.permissions.Count -ge 1) "permissions=$($adminRole.permissions.Count)"
Check '管理员角色统计到用户数' ($adminRole.user_count -ge 1) "user_count=$($adminRole.user_count)"

$r = Invoke-Api GET '/api/v1/admin/authz/policies' -Token $adminToken
Check '策略列表可查询' ($r.Status -eq 200 -and $r.Json.data.policies.Count -ge 16) "count=$($r.Json.data.policies.Count)"
Check '含角色继承规则' ($r.Json.data.stats.grouping_rules -ge 2) "grouping=$($r.Json.data.stats.grouping_rules)"

$r = Invoke-Api POST '/api/v1/admin/authz/policies' -Token $adminToken -Body @{ role = 'reviewer'; obj = '/api/v1/admin/smoke-probe'; act = 'GET' }
Check '可新增授权策略' ($r.Status -eq 201) "status=$($r.Status)"
$r = Invoke-Api DELETE '/api/v1/admin/authz/policies?role=reviewer&obj=/api/v1/admin/smoke-probe&act=GET' -Token $adminToken
Check '可删除授权策略' ($r.Status -eq 200) "status=$($r.Status)"

Section '审计留痕'

$r = Invoke-Api GET '/api/v1/admin/audit-logs?page_size=100' -Token $adminToken
Check '审计日志可查询' ($r.Status -eq 200 -and $r.Json.data.total -gt 0) "total=$($r.Json.data.total)"
$actions = @($r.Json.data.items.action)
Check '记录了登录审计' ($actions -contains 'auth.login') "actions=$($actions -join ',')"
Check '记录了公司创建审计' ($actions -contains 'company.create')
Check '记录了抓取审计' ($actions -contains 'crawl.run')
Check '记录了审核审计' (($actions -contains 'job.approve') -or ($actions -contains 'job.reject'))
Check '记录了角色变更审计' ($actions -contains 'user.set_roles')

$r = Invoke-Api GET '/api/v1/admin/audit-logs?action=company' -Token $adminToken
Check '审计日志支持动作模糊检索' ($r.Json.data.total -ge 1) "total=$($r.Json.data.total)"

Section '账号自助'

$r = Invoke-Api POST '/api/v1/me/password' -Token $adminToken -Body @{ old_password = 'WrongOldPassw0rd'; new_password = $script:TempAdminPassword }
Check '原密码错误时改密被拒绝' ($r.Status -eq 403 -or $r.Status -eq 400) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api PATCH '/api/v1/me' -Token $adminToken -Body @{ nickname = 'Smoke Admin' }
Check '可更新个人资料' ($r.Status -eq 200 -and $r.Json.data.nickname -eq 'Smoke Admin') "status=$($r.Status)"

$r = Invoke-Api POST '/api/v1/me/password' -Token $adminToken -Body @{ old_password = $AdminPassword; new_password = $script:TempAdminPassword }
Check '正确原密码可改密' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
Check '改密响应换发当前设备的新令牌' ([bool]$r.Json.data.token.access_token) "body=$($r.Raw)"
$adminToken = $r.Json.data.token.access_token
$adminRefresh = $r.Json.data.token.refresh_token

$r = Invoke-Api GET '/api/v1/me' -Token $adminToken
Check '改密后当前设备无需重新登录' ($r.Status -eq 200) "status=$($r.Status)"

$r = Invoke-Api POST '/api/v1/auth/login' @{ email = $AdminEmail; password = $script:TempAdminPassword }
Check '可用新密码登录' ($r.Status -eq 200) "status=$($r.Status)"
$adminToken = $r.Json.data.token.access_token
$adminRefresh = $r.Json.data.token.refresh_token

# ---------------------------------------------------------------- 清理
Section '清理与收尾'

# 先清理测试数据（此时令牌仍然有效）。
$r = Invoke-Api GET "/api/v1/admin/jobs?company_id=$companyId&page_size=50" -Token $adminToken
$cleaned = 0
if ($r.Status -eq 200) {
    foreach ($item in @($r.Json.data.items)) {
        $d = Invoke-Api DELETE "/api/v1/admin/jobs/$($item.id)" -Token $adminToken
        if ($d.Status -eq 200) { $cleaned++ }
    }
}
Check '测试岗位已清理' ($cleaned -ge 1) "cleaned=$cleaned status=$($r.Status)"

$r = Invoke-Api DELETE "/api/v1/admin/companies/$companyId" -Token $adminToken
Check '测试公司已清理' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"

# 清理本次创建的两个测试账号，保证脚本可以无限次重复运行且不留数据。
$r = Invoke-Api DELETE "/api/v1/admin/users/$newUserId" -Token $adminToken
Check '测试用户已清理（普通用户）' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"

$r = Invoke-Api DELETE "/api/v1/admin/users/$reviewerId" -Token $adminToken
Check '测试用户已清理（审核员）' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"

# 还原运行前备份的收集范围（而不是硬编码默认值），避免冲掉管理员的既有配置。
$r = Invoke-Api PUT '/api/v1/admin/crawl-scope' -Token $adminToken -Body @{
    job_types         = @($scopeBackup.job_types)
    industries        = @($scopeBackup.industries)
    employment_types  = @($scopeBackup.employment_types)
    locations         = @($scopeBackup.locations)
    keywords          = @($scopeBackup.keywords)
    exclude_keywords  = @($scopeBackup.exclude_keywords)
    min_quality_score = [int]$scopeBackup.min_quality_score
}
Check '收集范围已还原为运行前的配置' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"

# 恢复管理员原始密码，便于脚本重复运行。
$r = Invoke-Api POST '/api/v1/me/password' -Token $adminToken -Body @{ old_password = $script:TempAdminPassword; new_password = $AdminPassword }
Check '恢复管理员原始密码' ($r.Status -eq 200) "status=$($r.Status) body=$($r.Raw)"
$adminToken = $r.Json.data.token.access_token
$adminRefresh = $r.Json.data.token.refresh_token

$r = Invoke-Api POST '/api/v1/auth/logout' -Token $adminToken -Body @{ refresh_token = $adminRefresh }
Check '登出成功' ($r.Status -eq 200) "status=$($r.Status)"

$r = Invoke-Api POST '/api/v1/auth/refresh' @{ refresh_token = $adminRefresh }
Check '登出后刷新令牌已失效' ($r.Status -eq 401) "status=$($r.Status) code=$($r.Json.code)"

$r = Invoke-Api GET '/api/v1/me' -Token $adminToken
Check '登出后访问令牌立即失效' ($r.Status -eq 401) "status=$($r.Status)"

Write-Host ''
Write-Host "================================" -ForegroundColor Yellow
Write-Host "通过：$script:Pass  失败：$script:Fail  跳过：$script:Skip" -ForegroundColor $(if ($script:Fail -eq 0) { 'Green' } else { 'Red' })
Write-Host "公司ID：$companyId  抓取记录ID：$runId" -ForegroundColor DarkGray
Write-Host "================================" -ForegroundColor Yellow

if ($script:Fail -gt 0) { exit 1 }
exit 0
