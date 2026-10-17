// Package config 负责加载与校验应用配置。
//
// 优先级：内置默认值 < config.yaml < 环境变量（前缀 JOBSROBOT_）。
// 环境变量通过结构体上的 `env` 标签声明，例如 `env:"DB_HOST"` 对应 `JOBSROBOT_DB_HOST`。
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// EnvPrefix 是所有环境变量的统一前缀。
const EnvPrefix = "JOBSROBOT_"

// Config 是应用的全部配置。
type Config struct {
	App      App      `yaml:"app"`
	Server   Server   `yaml:"server"`
	Database Database `yaml:"database"`
	Redis    Redis    `yaml:"redis"`
	JWT      JWT      `yaml:"jwt"`
	Auth     Auth     `yaml:"auth"`
	Mail     Mail     `yaml:"mail"`
	Crawler  Crawler  `yaml:"crawler"`
	CORS     CORS     `yaml:"cors"`
	Log      Log      `yaml:"log"`
}

// App 是应用级元信息。
type App struct {
	Name      string `yaml:"name"      env:"APP_NAME"`
	Env       string `yaml:"env"       env:"APP_ENV"` // development | production | test
	Debug     bool   `yaml:"debug"     env:"APP_DEBUG"`
	APIPrefix string `yaml:"api_prefix" env:"API_PREFIX"`
	Timezone  string `yaml:"timezone"  env:"APP_TIMEZONE"`
}

// IsProduction 判断是否处于生产环境。
func (a App) IsProduction() bool { return a.Env == "production" }

// Server 是 HTTP 服务配置。
type Server struct {
	Host            string        `yaml:"host"              env:"SERVER_HOST"`
	Port            int           `yaml:"port"              env:"SERVER_PORT"`
	ReadTimeout     time.Duration `yaml:"read_timeout"      env:"SERVER_READ_TIMEOUT"`
	WriteTimeout    time.Duration `yaml:"write_timeout"     env:"SERVER_WRITE_TIMEOUT"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"  env:"SERVER_SHUTDOWN_TIMEOUT"`
	TrustedProxies  []string      `yaml:"trusted_proxies"   env:"SERVER_TRUSTED_PROXIES"`
}

// Addr 返回监听地址。
func (s Server) Addr() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

// Database 是 PostgreSQL 配置。
type Database struct {
	Host            string        `yaml:"host"              env:"DB_HOST"`
	Port            int           `yaml:"port"              env:"DB_PORT"`
	User            string        `yaml:"user"              env:"DB_USER"`
	Password        string        `yaml:"password"          env:"DB_PASSWORD"`
	Name            string        `yaml:"name"              env:"DB_NAME"`
	SSLMode         string        `yaml:"sslmode"           env:"DB_SSLMODE"`
	MaxOpenConns    int           `yaml:"max_open_conns"    env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConns    int           `yaml:"max_idle_conns"    env:"DB_MAX_IDLE_CONNS"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME"`
	AutoMigrate     bool          `yaml:"auto_migrate"      env:"DB_AUTO_MIGRATE"`
}

// DSN 返回 pgx 可用的连接串。
func (d Database) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

// Redis 是 Redis 配置。
type Redis struct {
	Addr         string        `yaml:"addr"           env:"REDIS_ADDR"`
	Password     string        `yaml:"password"       env:"REDIS_PASSWORD"`
	DB           int           `yaml:"db"             env:"REDIS_DB"`
	PoolSize     int           `yaml:"pool_size"      env:"REDIS_POOL_SIZE"`
	DialTimeout  time.Duration `yaml:"dial_timeout"   env:"REDIS_DIAL_TIMEOUT"`
	ReadTimeout  time.Duration `yaml:"read_timeout"   env:"REDIS_READ_TIMEOUT"`
	WriteTimeout time.Duration `yaml:"write_timeout"  env:"REDIS_WRITE_TIMEOUT"`
	KeyPrefix    string        `yaml:"key_prefix"     env:"REDIS_KEY_PREFIX"`
}

// JWT 是令牌签发配置。
type JWT struct {
	Secret     string        `yaml:"secret"      env:"JWT_SECRET"`
	Issuer     string        `yaml:"issuer"      env:"JWT_ISSUER"`
	AccessTTL  time.Duration `yaml:"access_ttl"  env:"JWT_ACCESS_TTL"`
	RefreshTTL time.Duration `yaml:"refresh_ttl" env:"JWT_REFRESH_TTL"`
}

// Auth 是认证相关的行为参数。
type Auth struct {
	BcryptCost           int           `yaml:"bcrypt_cost"            env:"AUTH_BCRYPT_COST"`
	CodeLength           int           `yaml:"code_length"            env:"AUTH_CODE_LENGTH"`
	CodeTTL              time.Duration `yaml:"code_ttl"               env:"AUTH_CODE_TTL"`
	CodeMaxAttempts      int           `yaml:"code_max_attempts"      env:"AUTH_CODE_MAX_ATTEMPTS"`
	CodeSendCooldown     time.Duration `yaml:"code_send_cooldown"     env:"AUTH_CODE_SEND_COOLDOWN"`
	CodeDailyLimit       int           `yaml:"code_daily_limit"       env:"AUTH_CODE_DAILY_LIMIT"`
	RegisterNeedVerify   bool          `yaml:"register_need_verify"   env:"AUTH_REGISTER_NEED_VERIFY"`
	LoginFailMax         int           `yaml:"login_fail_max"         env:"AUTH_LOGIN_FAIL_MAX"`
	LoginLockDuration    time.Duration `yaml:"login_lock_duration"    env:"AUTH_LOGIN_LOCK_DURATION"`
	PasswordMinLength    int           `yaml:"password_min_length"    env:"AUTH_PASSWORD_MIN_LENGTH"`
	ExposeCodeInResponse bool          `yaml:"expose_code_in_response" env:"AUTH_EXPOSE_CODE_IN_RESPONSE"`
	RateLimit            AuthRateLimit `yaml:"rate_limit"`
	Bootstrap            Bootstrap     `yaml:"bootstrap"`
	Setup                Setup         `yaml:"setup"`
}

// Bootstrap 是「首次启动自动创建初始管理员」的配置。
//
// 注意默认值是 false：有界面的部署应当走**安装向导**（前端 /setup →
// POST /api/v1/system/setup），由运维自己填写邮箱、密码与真实可用的 SMTP 参数，
// 而不是让服务在启动日志里吐一个随机口令。
//
// 之所以保留这一段：无界面场景（纯容器、CI、只跑 API 的镜像）没有 /setup
// 可用，此时把 auth.bootstrap.enabled 显式置 true 即可恢复启动自动建号，
// 凭据以 WARN 级打印在启动日志并落一份 0600 文件供运维查看。
//
// 幂等：只要系统中已存在一名「可用管理员」，本段配置不会改动任何既有账号。
type Bootstrap struct {
	// Enabled 是否启用启动自举；默认 false（首次安装请走安装向导，见 Setup）。
	//
	// 与 auth.setup.enabled 互斥：若两者同时为 true，启动自举会先建号，
	// 系统随即变成「已初始化」，安装向导自然失效（Config.Validate 会直接报错）。
	Enabled bool `yaml:"enabled" env:"AUTH_BOOTSTRAP_ENABLED"`
	// Email 初始管理员邮箱；默认 admin@jobs-robot.local。
	Email string `yaml:"email" env:"AUTH_BOOTSTRAP_EMAIL"`
	// Password 初始管理员密码；留空表示自动生成随机强密码。
	//
	// 安全提示：在此处配置的密码会被写入本文件/环境变量，因此
	// 自举日志只打印「(来自配置，未打印)」，不会把运维自己设置的密码写进日志。
	Password string `yaml:"password" env:"AUTH_BOOTSTRAP_PASSWORD"`
	// Nickname 初始管理员昵称；默认「初始管理员」。
	Nickname string `yaml:"nickname" env:"AUTH_BOOTSTRAP_NICKNAME"`
	// CredentialFile 凭据文件路径（0600）；留空表示不写文件。默认 var/bootstrap-admin.txt。
	CredentialFile string `yaml:"credential_file" env:"AUTH_BOOTSTRAP_CREDENTIAL_FILE"`
}

// minBootstrapPasswordLength 是自举密码的最小长度。
//
// 为什么不直接调用 auth.ValidatePasswordStrength：internal/config 是最底层的
// 配置包，internal/auth 会反向依赖它（auth 的令牌/验证码都吃 config 类型），
// 一旦此处 import auth 就会形成包循环依赖。这里保留一条与之等价的最小校验
// （长度下限 + 首尾无空白 + 同时含字母与数字），完整规则仍由
// auth.ValidatePasswordStrength 在写入密码时把关。
const minBootstrapPasswordLength = 8

// Validate 校验自举配置。
func (b Bootstrap) Validate() error {
	if !b.Enabled {
		return nil
	}

	var problems []string
	email := strings.TrimSpace(b.Email)
	if email == "" {
		problems = append(problems, "auth.bootstrap.email 不能为空（初始管理员需要可登录的邮箱）")
	} else if !strings.Contains(email, "@") {
		problems = append(problems, "auth.bootstrap.email 需要是邮箱格式（含 @）")
	}

	if password := b.Password; password != "" {
		if len([]rune(password)) < minBootstrapPasswordLength {
			problems = append(problems, "auth.bootstrap.password 长度至少 8 位（留空则自动生成随机强密码）")
		}
		if password != strings.TrimSpace(password) {
			problems = append(problems, "auth.bootstrap.password 首尾不能包含空白字符")
		}
		var hasLetter, hasDigit bool
		for _, r := range password {
			switch {
			case unicode.IsLetter(r):
				hasLetter = true
			case unicode.IsDigit(r):
				hasDigit = true
			}
		}
		if !hasLetter || !hasDigit {
			problems = append(problems, "auth.bootstrap.password 需同时包含字母与数字")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("初始管理员自举配置有误：%s", strings.Join(problems, "；"))
	}
	return nil
}

// Setup 是「首次安装向导」（前端 /setup ←→ POST /api/v1/system/setup）的配置。
//
// 与 Bootstrap 的区别：
//   - Bootstrap 是**服务端**在启动时自动建号，凭据由系统生成，适合无界面部署；
//   - Setup 是**运维**在浏览器里一次性完成建号 + 配置并验证 SMTP，适合有界面部署。
//
// 安装向导只在「系统尚无可用管理员」时可用：一旦存在管理员即永久失效（返回 409），
// 因此它不是一个可以反复调用的建号接口。
type Setup struct {
	// Enabled 是否开放安装向导接口；默认 true。
	//
	// 置 false 会同时让 /system/status 的 setup_available 恒为 false，
	// 前端因此不会强制跳转 /setup——适用于「由外部系统预置账号」的部署。
	Enabled bool `yaml:"enabled" env:"AUTH_SETUP_ENABLED"`
	// TestMailTo 是测试邮件的默认收件人；留空表示由请求体指定。
	//
	// 存在这个配置是因为有些部署希望测试邮件固定发到运维邮箱，
	// 而不是发到刚刚创建的管理员邮箱（后者可能是待验证的企业邮箱）。
	TestMailTo string `yaml:"test_mail_to" env:"AUTH_SETUP_TEST_MAIL_TO"`
}

// AuthRateLimit 是认证接口的 IP 维度限流配置。
//
// 为什么必须可配置：不同部署形态对认证接口的压力差异极大
// （本地联调、自动化测试、生产公网），写死在代码里会导致
// 「测试环境取不到验证码」或「生产环境形同不限流」。
// 阈值为 0 表示不限流。
type AuthRateLimit struct {
	Enabled bool `yaml:"enabled" env:"AUTH_RL_ENABLED"`

	// SendCode 对应 POST /auth/codes 与找回密码的验证码发送
	SendCodeLimit  int           `yaml:"send_code_limit"  env:"AUTH_RL_SEND_CODE_LIMIT"`
	SendCodeWindow time.Duration `yaml:"send_code_window" env:"AUTH_RL_SEND_CODE_WINDOW"`

	RegisterLimit  int           `yaml:"register_limit"  env:"AUTH_RL_REGISTER_LIMIT"`
	RegisterWindow time.Duration `yaml:"register_window" env:"AUTH_RL_REGISTER_WINDOW"`

	LoginLimit  int           `yaml:"login_limit"  env:"AUTH_RL_LOGIN_LIMIT"`
	LoginWindow time.Duration `yaml:"login_window" env:"AUTH_RL_LOGIN_WINDOW"`

	ForgotLimit  int           `yaml:"forgot_limit"  env:"AUTH_RL_FORGOT_LIMIT"`
	ForgotWindow time.Duration `yaml:"forgot_window" env:"AUTH_RL_FORGOT_WINDOW"`

	ResetLimit  int           `yaml:"reset_limit"  env:"AUTH_RL_RESET_LIMIT"`
	ResetWindow time.Duration `yaml:"reset_window" env:"AUTH_RL_RESET_WINDOW"`

	// Setup 对应 POST /system/setup 与 POST /system/setup/mail/test。
	//
	// 为什么也要限流：这两个接口虽然在「已有管理员」后返回 409 / 无副作用，
	// 但在系统尚未初始化时是**公开可调用的**——不限流就等于把管理员账号
	// 创建接口暴露给任意来源，也给测试发信留下了被当作邮件炮台的可能。
	SetupLimit  int           `yaml:"setup_limit"  env:"AUTH_RL_SETUP_LIMIT"`
	SetupWindow time.Duration `yaml:"setup_window" env:"AUTH_RL_SETUP_WINDOW"`
}

// Mail 是邮件发送配置。
type Mail struct {
	Driver string `yaml:"driver" env:"MAIL_DRIVER"` // console | smtp
	// SystemFromName 是所有「系统自动邮件」的发件人显示名（品牌化的系统通知名）。
	//
	// 它与 FromName 分开：FromName 是邮件通道的通用兜底显示名，
	// SystemFromName 才是收件人在收件箱里看到的系统通知身份。
	// 为空时回退 FromName，两者都为空再用 no-reply。
	SystemFromName string `yaml:"system_from_name" env:"MAIL_SYSTEM_FROM_NAME"`
	FromName       string `yaml:"from_name"        env:"MAIL_FROM_NAME"`
	FromAddress    string `yaml:"from_address"     env:"MAIL_FROM_ADDRESS"`
	SMTP           SMTP   `yaml:"smtp"`
}

// DefaultSystemFromName 是系统邮件的默认发件人显示名。
//
// 中间是 U+00B7 中点（·），两侧各一个空格；不是 ASCII 句点。
const DefaultSystemFromName = "直招好工作 · 系统通知"

// fallbackFromName 是 system_from_name 与 from_name 都为空时的安全兜底。
const fallbackFromName = "no-reply"

// EffectiveSystemFromName 解析系统邮件实际使用的发件人显示名。
//
// 优先级：mail.system_from_name > mail.from_name > no-reply。
// 单独做成方法而不是各调用点自己判空：From 头、后台设置页、测试发信必须同源。
func (m Mail) EffectiveSystemFromName() string {
	if name := strings.TrimSpace(m.SystemFromName); name != "" {
		return name
	}
	if name := strings.TrimSpace(m.FromName); name != "" {
		return name
	}
	return fallbackFromName
}

// SMTP 是 SMTP 服务参数。
type SMTP struct {
	Host     string `yaml:"host"     env:"MAIL_SMTP_HOST"`
	Port     int    `yaml:"port"     env:"MAIL_SMTP_PORT"`
	Username string `yaml:"username" env:"MAIL_SMTP_USERNAME"`
	Password string `yaml:"password" env:"MAIL_SMTP_PASSWORD"`
	// TLS 为 true 时使用隐式 TLS（465 端口），否则按 STARTTLS 升级。
	TLS bool `yaml:"tls" env:"MAIL_SMTP_TLS"`
}

// Validate 校验邮件配置。
//
// 抽成独立方法的原因：管理员可以在后台在线修改邮件配置，
// 那条写入路径必须与服务启动时使用同一套校验规则，否则会出现
// 「启动能过、后台保存能过、但发信必失败」的配置。
func (m Mail) Validate() error {
	switch m.Driver {
	case "console":
		return nil
	case "smtp":
		var problems []string
		if strings.TrimSpace(m.SMTP.Host) == "" {
			problems = append(problems, "smtp 主机不能为空")
		}
		if m.SMTP.Port <= 0 || m.SMTP.Port > 65535 {
			problems = append(problems, "smtp 端口必须在 1~65535 之间")
		}
		if strings.TrimSpace(m.FromAddress) == "" {
			problems = append(problems, "发件人邮箱不能为空")
		} else if !strings.Contains(m.FromAddress, "@") {
			problems = append(problems, "发件人邮箱格式不正确")
		}
		// 只填用户名不填密码（或反之）几乎总是配置失误，提前拦下来。
		hasUser := strings.TrimSpace(m.SMTP.Username) != ""
		hasPass := m.SMTP.Password != ""
		if hasUser != hasPass {
			problems = append(problems, "SMTP 用户名与密码需要同时填写（若服务器无需认证请都留空）")
		}
		if len(problems) > 0 {
			return fmt.Errorf("邮件配置有误：%s", strings.Join(problems, "；"))
		}
		return nil
	default:
		return fmt.Errorf("邮件驱动只支持 console/smtp，当前为 %q", m.Driver)
	}
}

// Crawler 是爬虫子系统的全部配置。
type Crawler struct {
	Enabled         bool          `yaml:"enabled"            env:"CRAWLER_ENABLED"`
	Schedule        string        `yaml:"schedule"           env:"CRAWLER_SCHEDULE"`
	Workers         int           `yaml:"workers"            env:"CRAWLER_WORKERS"`
	PerHostLimit    int           `yaml:"per_host_limit"     env:"CRAWLER_PER_HOST_LIMIT"`
	RequestTimeout  time.Duration `yaml:"request_timeout"    env:"CRAWLER_REQUEST_TIMEOUT"`
	Delay           time.Duration `yaml:"delay"              env:"CRAWLER_DELAY"`
	RandomDelay     time.Duration `yaml:"random_delay"       env:"CRAWLER_RANDOM_DELAY"`
	MaxRetries      int           `yaml:"max_retries"        env:"CRAWLER_MAX_RETRIES"`
	MaxDepth        int           `yaml:"max_depth"          env:"CRAWLER_MAX_DEPTH"`
	MaxPagesPerSite int           `yaml:"max_pages_per_site" env:"CRAWLER_MAX_PAGES_PER_SITE"`
	RespectRobots   bool          `yaml:"respect_robots"     env:"CRAWLER_RESPECT_ROBOTS"`
	RobotsTTL       time.Duration `yaml:"robots_ttl"         env:"CRAWLER_ROBOTS_TTL"`
	LockTTL         time.Duration `yaml:"lock_ttl"           env:"CRAWLER_LOCK_TTL"`
	// JobExpireAfter：连续多久未在来源页面上再次出现，则视为已下线。
	JobExpireAfter time.Duration `yaml:"job_expire_after" env:"CRAWLER_JOB_EXPIRE_AFTER"`
	Identity       Identity      `yaml:"identity"`
	Headless       Headless      `yaml:"headless"`
	Scope          Scope         `yaml:"scope"`
	Storage        Storage       `yaml:"storage"`
}

// Identity 描述爬虫对外声明的机器人身份，用于构造标准请求头。
//
// 这是「按标准填充身份信息为 AI 机器人访问」的落点：
// 每一个出站请求都会带上 User-Agent、From、X-Robot-* 等头，站长可据此识别与联系。
type Identity struct {
	BotName      string `yaml:"bot_name"      env:"CRAWLER_BOT_NAME"`
	BotVersion   string `yaml:"bot_version"   env:"CRAWLER_BOT_VERSION"`
	ContactEmail string `yaml:"contact_email" env:"CRAWLER_CONTACT_EMAIL"`
	Homepage     string `yaml:"homepage"      env:"CRAWLER_HOMEPAGE"`
	Purpose      string `yaml:"purpose"       env:"CRAWLER_PURPOSE"`
	// IsAI 声明抓取方为 AI 机器人（写入 X-Robot-AI 头）。
	IsAI bool `yaml:"is_ai" env:"CRAWLER_IS_AI"`
	// RespectNoAI 表示若站点通过 noai/nogptbot 元标签拒绝 AI 抓取，则主动跳过。
	RespectNoAI bool   `yaml:"respect_no_ai" env:"CRAWLER_RESPECT_NO_AI"`
	Language    string `yaml:"language"    env:"CRAWLER_LANGUAGE"`
}

// Headless 是无头浏览器兜底配置。
type Headless struct {
	Enabled bool `yaml:"enabled" env:"CRAWLER_HEADLESS_ENABLED"`
	// FallbackMinLength：静态抓取正文短于该长度时切换到无头渲染。
	FallbackMinLength int           `yaml:"fallback_min_length" env:"CRAWLER_HEADLESS_FALLBACK_MIN_LENGTH"`
	Timeout           time.Duration `yaml:"timeout"             env:"CRAWLER_HEADLESS_TIMEOUT"`
	ExecutablePath    string        `yaml:"executable_path"     env:"CRAWLER_HEADLESS_EXECUTABLE_PATH"`
	WaitAfterLoad     time.Duration `yaml:"wait_after_load"     env:"CRAWLER_HEADLESS_WAIT_AFTER_LOAD"`
	MaxConcurrent     int           `yaml:"max_concurrent"      env:"CRAWLER_HEADLESS_MAX_CONCURRENT"`
}

// Scope 规定爬虫只收集哪些工作类型与行业（可被数据库中的 Setting 覆盖）。
type Scope struct {
	JobTypes        []string `yaml:"job_types"        env:"CRAWLER_SCOPE_JOB_TYPES"`
	Industries      []string `yaml:"industries"       env:"CRAWLER_SCOPE_INDUSTRIES"`
	EmploymentTypes []string `yaml:"employment_types" env:"CRAWLER_SCOPE_EMPLOYMENT_TYPES"`
	Locations       []string `yaml:"locations"        env:"CRAWLER_SCOPE_LOCATIONS"`
	Keywords        []string `yaml:"keywords"         env:"CRAWLER_SCOPE_KEYWORDS"`
	ExcludeKeywords []string `yaml:"exclude_keywords" env:"CRAWLER_SCOPE_EXCLUDE_KEYWORDS"`
	// MinQualityScore：低于该质量分的岗位不落库。
	MinQualityScore int `yaml:"min_quality_score" env:"CRAWLER_SCOPE_MIN_QUALITY_SCORE"`
}

// Storage 是抓取快照存储配置。
type Storage struct {
	SnapshotHTML bool   `yaml:"snapshot_html" env:"CRAWLER_SNAPSHOT_HTML"`
	Dir          string `yaml:"dir"           env:"CRAWLER_SNAPSHOT_DIR"`
}

// CORS 是跨域配置，供后续前端接入。
type CORS struct {
	Enabled       bool     `yaml:"enabled"         env:"CORS_ENABLED"`
	AllowOrigins  []string `yaml:"allow_origins"   env:"CORS_ALLOW_ORIGINS"`
	AllowMethods  []string `yaml:"allow_methods"   env:"CORS_ALLOW_METHODS"`
	AllowHeaders  []string `yaml:"allow_headers"   env:"CORS_ALLOW_HEADERS"`
	ExposeHeaders []string `yaml:"expose_headers"  env:"CORS_EXPOSE_HEADERS"`
	AllowCred     bool     `yaml:"allow_credentials" env:"CORS_ALLOW_CREDENTIALS"`
	MaxAge        int      `yaml:"max_age"         env:"CORS_MAX_AGE"`
}

// Log 是日志配置。
type Log struct {
	Level     string `yaml:"level"       env:"LOG_LEVEL"`  // debug | info | warn | error
	Format    string `yaml:"format"      env:"LOG_FORMAT"` // json | console
	Output    string `yaml:"output"      env:"LOG_OUTPUT"` // stdout | 文件路径
	AccessLog bool   `yaml:"access_log"  env:"LOG_ACCESS"`
}

// Default 返回内置默认配置，保证不提供 config.yaml 也能启动。
func Default() *Config {
	return &Config{
		App: App{
			Name:      "jobs-robot",
			Env:       "development",
			Debug:     true,
			APIPrefix: "/api/v1",
			Timezone:  "Asia/Shanghai",
		},
		Server: Server{
			Host:            "0.0.0.0",
			Port:            8090,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    60 * time.Second,
			ShutdownTimeout: 15 * time.Second,
		},
		Database: Database{
			Host:            "127.0.0.1",
			Port:            55432,
			User:            "jobsrobot",
			Password:        "jobsrobot",
			Name:            "jobsrobot",
			SSLMode:         "disable",
			MaxOpenConns:    30,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			AutoMigrate:     true,
		},
		Redis: Redis{
			Addr:         "127.0.0.1:56379",
			DB:           0,
			PoolSize:     20,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			KeyPrefix:    "jobsrobot:",
		},
		JWT: JWT{
			Issuer:     "jobs-robot",
			AccessTTL:  2 * time.Hour,
			RefreshTTL: 7 * 24 * time.Hour,
		},
		Auth: Auth{
			BcryptCost:           12,
			CodeLength:           6,
			CodeTTL:              10 * time.Minute,
			CodeMaxAttempts:      5,
			CodeSendCooldown:     60 * time.Second,
			CodeDailyLimit:       20,
			RegisterNeedVerify:   true,
			LoginFailMax:         5,
			LoginLockDuration:    15 * time.Minute,
			PasswordMinLength:    8,
			ExposeCodeInResponse: false,
			Bootstrap: Bootstrap{
				// 默认关闭：有界面的部署走安装向导（前端 /setup），
				// 由运维自己设定密码并现场验证 SMTP，而不是收一个随机口令。
				// 无界面/容器部署可显式置 true 恢复启动自动建号。
				Enabled:        false,
				Email:          "admin@jobs-robot.local",
				Password:       "", // 留空 → 启动时生成随机强密码
				Nickname:       "初始管理员",
				CredentialFile: "var/bootstrap-admin.txt",
			},
			Setup: Setup{
				// 默认开放：全新部署第一次打开前端就应该能进安装向导。
				Enabled:    true,
				TestMailTo: "",
			},
			RateLimit: AuthRateLimit{
				Enabled:        true,
				SendCodeLimit:  10,
				SendCodeWindow: time.Hour,
				RegisterLimit:  20,
				RegisterWindow: time.Hour,
				LoginLimit:     30,
				LoginWindow:    10 * time.Minute,
				ForgotLimit:    10,
				ForgotWindow:   time.Hour,
				ResetLimit:     20,
				ResetWindow:    time.Hour,
				// 安装向导：10 次/小时。首次安装一次就能过，留足重试与
				// 「测试邮件发失败 → 改参数重试」的余量即可。
				SetupLimit:  10,
				SetupWindow: time.Hour,
			},
		},
		Mail: Mail{
			Driver:         "console",
			SystemFromName: DefaultSystemFromName,
			FromName:       "直招好工作",
			FromAddress:    "no-reply@jobs-robot.local",
			SMTP:           SMTP{Port: 465, TLS: true},
		},
		Crawler: Crawler{
			Enabled:         true,
			Schedule:        "0 */6 * * *",
			Workers:         4,
			PerHostLimit:    1,
			RequestTimeout:  30 * time.Second,
			Delay:           2 * time.Second,
			RandomDelay:     1 * time.Second,
			MaxRetries:      3,
			MaxDepth:        3,
			MaxPagesPerSite: 200,
			RespectRobots:   true,
			RobotsTTL:       12 * time.Hour,
			LockTTL:         30 * time.Minute,
			JobExpireAfter:  30 * 24 * time.Hour,
			Identity: Identity{
				BotName:      "JobsRobotBot",
				BotVersion:   "1.0",
				ContactEmail: "crawler@jobs-robot.local",
				Homepage:     "https://jobs-robot.local/bot",
				Purpose:      "聚合企业官网直招岗位信息，供求职者检索",
				IsAI:         true,
				RespectNoAI:  true,
				Language:     "zh-CN,zh;q=0.9,en;q=0.8",
			},
			Headless: Headless{
				Enabled:           true,
				FallbackMinLength: 800,
				Timeout:           45 * time.Second,
				WaitAfterLoad:     1500 * time.Millisecond,
				MaxConcurrent:     2,
			},
			Scope: Scope{
				JobTypes:        []string{"技术", "产品", "设计", "运营", "市场", "职能"},
				Industries:      []string{"互联网", "人工智能", "金融科技", "智能制造", "新能源"},
				EmploymentTypes: []string{"全职", "实习", "校招", "社招"},
				MinQualityScore: 40,
			},
			Storage: Storage{
				SnapshotHTML: false,
				Dir:          "var/snapshots",
			},
		},
		CORS: CORS{
			Enabled:      true,
			AllowOrigins: []string{"http://localhost:5173", "http://127.0.0.1:5173"},
			AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-Id"},
			MaxAge:       86400,
		},
		Log: Log{Level: "info", Format: "console", Output: "stdout", AccessLog: true},
	}
}

// Load 读取配置文件（可为空字符串表示只用默认值+环境变量），再叠加环境变量并校验。
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		raw, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(raw, cfg); err != nil {
				return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
			}
		case errors.Is(err, os.ErrNotExist):
			// 允许缺省：使用默认值。
		default:
			return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
		}
	}

	if err := ApplyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ApplyEnv 把 JOBSROBOT_ 前缀的环境变量覆盖到 cfg 上。
func ApplyEnv(cfg *Config) error {
	return walkEnv(reflect.ValueOf(cfg).Elem(), EnvPrefix)
}

func walkEnv(v reflect.Value, prefix string) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)

		// 嵌套结构体（且非时间类型）递归处理。
		if fv.Kind() == reflect.Struct && fv.Type() != reflect.TypeOf(time.Duration(0)) && fv.Type() != reflect.TypeOf(time.Time{}) {
			if err := walkEnv(fv, prefix); err != nil {
				return err
			}
			continue
		}

		key := field.Tag.Get("env")
		if key == "" {
			continue
		}
		raw, ok := os.LookupEnv(prefix + key)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		if err := setField(fv, raw, prefix+key); err != nil {
			return err
		}
	}
	return nil
}

func setField(fv reflect.Value, raw, envName string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("环境变量 %s 需要布尔值，得到 %q", envName, raw)
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// time.Duration 底层是 int64，需按 Duration 解析。
		if fv.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf("环境变量 %s 需要时长（如 30s/2h），得到 %q", envName, raw)
			}
			fv.SetInt(int64(d))
			return nil
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("环境变量 %s 需要整数，得到 %q", envName, raw)
		}
		fv.SetInt(n)
	case reflect.Slice:
		if fv.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("环境变量 %s 暂不支持该切片类型 %s", envName, fv.Type())
		}
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		fv.Set(reflect.ValueOf(out))
	default:
		return fmt.Errorf("环境变量 %s 命中不支持的字段类型 %s", envName, fv.Type())
	}
	return nil
}

// Validate 校验关键配置，尽早暴露配置错误。
func (c *Config) Validate() error {
	var problems []string

	switch c.App.Env {
	case "development", "production", "test":
	default:
		problems = append(problems, fmt.Sprintf("app.env 必须是 development/production/test 之一，当前为 %q", c.App.Env))
	}
	if c.App.APIPrefix == "" || !strings.HasPrefix(c.App.APIPrefix, "/") {
		problems = append(problems, "app.api_prefix 必须以 / 开头")
	}
	if c.App.Timezone != "" {
		if _, err := time.LoadLocation(c.App.Timezone); err != nil {
			problems = append(problems, fmt.Sprintf("app.timezone 无效: %v（Windows 下需确保已安装时区数据，可保持 Asia/Shanghai）", err))
		}
	}

	if c.Database.Host == "" || c.Database.Name == "" || c.Database.User == "" {
		problems = append(problems, "database.host/name/user 不能为空")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		problems = append(problems, "database.port 非法")
	}
	if c.Redis.Addr == "" {
		problems = append(problems, "redis.addr 不能为空")
	}

	if c.JWT.Secret == "" {
		problems = append(problems, "jwt.secret 不能为空")
	} else if len(c.JWT.Secret) < 32 {
		problems = append(problems, "jwt.secret 长度至少 32 字节，请使用强随机值")
	} else if c.App.IsProduction() && strings.Contains(c.JWT.Secret, "change-me") {
		problems = append(problems, "生产环境必须替换默认的 jwt.secret")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.RefreshTTL <= 0 {
		problems = append(problems, "jwt.access_ttl / jwt.refresh_ttl 必须为正数")
	}
	if c.JWT.RefreshTTL <= c.JWT.AccessTTL {
		problems = append(problems, "jwt.refresh_ttl 应大于 jwt.access_ttl")
	}

	if c.Auth.BcryptCost < 10 || c.Auth.BcryptCost > 16 {
		problems = append(problems, "auth.bcrypt_cost 建议在 10~16 之间")
	}
	if c.Auth.CodeLength < 4 || c.Auth.CodeLength > 10 {
		problems = append(problems, "auth.code_length 建议在 4~10 之间")
	}
	if c.Auth.PasswordMinLength < 8 {
		problems = append(problems, "auth.password_min_length 不得小于 8")
	}

	// 初始管理员自举（auth.bootstrap.*）：校验交给 Bootstrap.Validate，
	// 便于单测直接覆盖「启用/停用 + 密码留空/给定」的组合。
	if err := c.Auth.Bootstrap.Validate(); err != nil {
		problems = append(problems, err.Error())
	}

	// 安装向导（auth.setup.*）与自举互斥：两者同时开启时，启动自举会先建号，
	// 系统立刻变成「已初始化」，安装向导就永远用不上了——这属于配置错误，
	// 与其让运维对着一个「打开就 409」的向导页困惑，不如启动时直接说清楚。
	if c.Auth.Setup.Enabled && c.Auth.Bootstrap.Enabled {
		problems = append(problems,
			"auth.setup.enabled 与 auth.bootstrap.enabled 不能同时为 true："+
				"启动自举会先创建管理员，导致安装向导（/setup）永久失效；"+
				"有界面部署请保持 auth.bootstrap.enabled=false，无界面部署请关闭 auth.setup.enabled")
	}

	// 测试邮件默认收件人是可选项，填了就必须是邮箱格式。
	if to := strings.TrimSpace(c.Auth.Setup.TestMailTo); to != "" && !strings.Contains(to, "@") {
		problems = append(problems, "auth.setup.test_mail_to 需要是邮箱格式（含 @），留空表示由请求指定")
	}

	// 邮件配置校验抽成 Mail.Validate()，供「后台在线修改邮件配置」复用同一套规则。
	if err := c.Mail.Validate(); err != nil {
		problems = append(problems, err.Error())
	}

	if c.Crawler.Enabled {
		if c.Crawler.Identity.BotName == "" {
			problems = append(problems, "crawler.identity.bot_name 不能为空：必须以标准身份声明 AI 机器人")
		}
		if c.Crawler.Identity.ContactEmail == "" {
			problems = append(problems, "crawler.identity.contact_email 不能为空：需给站长提供可联系邮箱")
		}
		if c.Crawler.Workers <= 0 {
			problems = append(problems, "crawler.workers 必须大于 0")
		}
		if c.Crawler.Delay <= 0 {
			problems = append(problems, "crawler.delay 必须大于 0：请对目标站点保持友好抓取频率")
		}
		if c.Crawler.MaxPagesPerSite <= 0 {
			problems = append(problems, "crawler.max_pages_per_site 必须大于 0")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("配置校验失败:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}
