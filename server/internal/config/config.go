package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server    ServerConfig    `toml:"server"`
	Database  DatabaseConfig  `toml:"database"`
	Auth      AuthConfig      `toml:"auth"`
	Log       LogConfig       `toml:"log"`
	Telegram  TelegramConfig  `toml:"telegram"`
	Scheduler SchedulerConfig `toml:"scheduler"`

	// OAuth 不读 TOML —— 全部走环境变量(.env 文件 + 进程环境)。
	// 见 LoadOAuthFromEnv 与 .env.example。
	OAuth OAuthConfig `toml:"-"`
}

type ServerConfig struct {
	Listen                 string `toml:"listen"`
	ShutdownTimeoutSeconds int    `toml:"shutdown_timeout_seconds"`
	WriteTimeoutSeconds    int    `toml:"write_timeout_seconds"`
	ReadTimeoutSeconds     int    `toml:"read_timeout_seconds"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type AuthConfig struct {
	JWTSecret         string `toml:"jwt_secret"`
	AccessTTLSeconds  int    `toml:"access_ttl_seconds"`
	RefreshTTLSeconds int    `toml:"refresh_ttl_seconds"`
	BcryptCost        int    `toml:"bcrypt_cost"`
}

type LogConfig struct {
	Level string `toml:"level"`
}

// OAuthConfig 接入外部 OAuth2 / OpenID Connect 认证中心。
//
// 这一段配置完全不走 TOML —— URL、client_id、client_secret 等都比较敏感(尤其
// secret),按惯例放到 .env 文件 / 进程环境变量里,跟 git 仓库分离。
// 字段映射见 LoadOAuthFromEnv 与 .env.example。
//
// 当 Enabled = true 时:
//   - 关闭 /api/auth/register 与 /api/auth/login 端点(返回 403)。
//   - /api/auth/oauth/start 把用户重定向到 AuthorizeURL。
//   - /api/auth/oauth/callback 接收授权码,调用 TokenURL 换 access_token,
//     再调用 UserInfoURL 拉用户资料,在本库内 upsert 用户后签发本服务的 JWT。
//
// 各 URL 必须按你认证中心实际暴露的端点填写;不同实现 (Hydra / Keycloak /
// Authelia / 自研) 路径不同,本服务不做猜测。
type OAuthConfig struct {
	// 是否启用 OAuth 登录。false 时退化为本地邮箱密码注册/登录。
	Enabled bool
	// 提供方标识符,会写到 users.oauth_provider 列。建议用域名,例如
	// "teamcy.eu.cc"。改了之后已绑定的用户会被视为新用户(因此一旦上线就别动)。
	Provider string
	// 授权端点 (Authorization Endpoint)。浏览器会被 302 到这里。
	AuthorizeURL string
	// 令牌端点 (Token Endpoint)。服务端用 client_id/client_secret + code 换 access_token。
	TokenURL string
	// 用户信息端点 (UserInfo Endpoint)。带 Bearer access_token 调用,返回 sub/email/name 等。
	UserInfoURL string
	// 在认证中心创建客户端时拿到的 Client ID。
	ClientID string
	// 在认证中心创建客户端时拿到的 Client Secret。务必当作密码保管。
	ClientSecret string
	// 我方接收回调的完整 URL。必须与认证中心客户端配置里的「回调 URI」完全一致(逐字符)。
	// 形如 https://taskflow.your-domain.com/api/auth/oauth/callback
	RedirectURL string
	// 授权请求的 scope 列表;留空时使用 ["openid","profile","email"]。
	Scopes []string
	// 服务端处理完 OAuth 后,把用户重定向回前端这个 URL,并在 hash 中带 handoff_code。
	// 形如 https://taskflow.your-domain.com/oauth/callback
	// 留空时取 RedirectURL 同源 + /oauth/callback。
	FrontendRedirectURL string
	// userinfo 响应里取「邮箱」用的 JSON 字段名,默认 "email"。
	EmailField string
	// userinfo 响应里取「展示名」用的 JSON 字段名,默认 "name"(再退到 "preferred_username")。
	NameField string
	// userinfo 响应里取「主体标识 (sub)」用的 JSON 字段名,默认 "sub"(再退到 "id")。
	SubjectField string
}

type TelegramConfig struct {
	// Bot 的 token,从 @BotFather 拿。空时禁用所有 telegram 功能。
	BotToken string `toml:"bot_token"`
	// Bot 的 username(不带 @),用来给客户端拼 deep-link。
	BotUsername string `toml:"bot_username"`
	// 设置 webhook 时使用的 secret_token。Telegram 在每次回调请求中通过
	// X-Telegram-Bot-Api-Secret-Token 头回传,我们用 constant-time 比较来验证。
	WebhookSecret string `toml:"webhook_secret"`
	// bind_token 的 TTL,秒。0 取默认 600(10 分钟)。
	BindTokenTTLSeconds int `toml:"bind_token_ttl_seconds"`
}

type SchedulerConfig struct {
	// 扫描周期(秒),0 取默认 5。
	TickIntervalSeconds int `toml:"tick_interval_seconds"`
	// 单次最多处理多少条 due 提醒,0 取默认 200。
	BatchSize int `toml:"batch_size"`
	// 关掉调度器(开发/调试用)。
	Disabled bool `toml:"disabled"`
}

// Load 读取配置文件并应用默认值与基本校验。
//
// OAuth 配置不在 TOML 里,而是从环境变量(可选地通过 .env 文件)读取 ——
// 调用方负责在调用 Load 之前先 LoadEnvFile(),见 cmd/server/main.go。
//
// 此外,以下敏感字段也允许通过环境变量覆盖 TOML 中的值,让 docker / k8s 部署
// 时不必把秘密刻进镜像或挂载文件:
//
//	TASKFLOW_JWT_SECRET     -> [auth] jwt_secret
//	TASKFLOW_DB_PATH        -> [database] path
//	TASKFLOW_LISTEN         -> [server] listen
//	TASKFLOW_LOG_LEVEL      -> [log] level
//	TASKFLOW_TG_BOT_TOKEN   -> [telegram] bot_token
//	TASKFLOW_TG_BOT_USER    -> [telegram] bot_username
//	TASKFLOW_TG_WEBHOOK_SEC -> [telegram] webhook_secret
//
// 任意字段非空时即覆盖。空字符串视为"沿用 TOML 值"。
func Load(path string) (*Config, error) {
	cfg := defaults()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	applyEnvOverrides(cfg)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	// OAuth 块单独从环境变量加载并校验。
	oa, err := LoadOAuthFromEnv()
	if err != nil {
		return nil, err
	}
	cfg.OAuth = oa
	// 确保数据库目录存在
	if dir := filepath.Dir(cfg.Database.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return cfg, nil
}

// applyEnvOverrides 把 TASKFLOW_* 系列环境变量的非空值覆盖到 Config 中。
// 见 Load 文档注释里的列表。
func applyEnvOverrides(c *Config) {
	if v := os.Getenv("TASKFLOW_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	if v := os.Getenv("TASKFLOW_DB_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("TASKFLOW_LISTEN"); v != "" {
		c.Server.Listen = v
	}
	if v := os.Getenv("TASKFLOW_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("TASKFLOW_TG_BOT_TOKEN"); v != "" {
		c.Telegram.BotToken = v
	}
	if v := os.Getenv("TASKFLOW_TG_BOT_USER"); v != "" {
		c.Telegram.BotUsername = v
	}
	if v := os.Getenv("TASKFLOW_TG_WEBHOOK_SEC"); v != "" {
		c.Telegram.WebhookSecret = v
	}
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:                 "127.0.0.1:8080",
			ShutdownTimeoutSeconds: 15,
			WriteTimeoutSeconds:    30,
			ReadTimeoutSeconds:     30,
		},
		Database: DatabaseConfig{Path: "data/taskflow.db"},
		Auth: AuthConfig{
			AccessTTLSeconds:  900,
			RefreshTTLSeconds: 2592000,
			BcryptCost:        11,
		},
		Log: LogConfig{Level: "info"},
		Telegram: TelegramConfig{
			BindTokenTTLSeconds: 600,
		},
		Scheduler: SchedulerConfig{
			TickIntervalSeconds: 5,
			BatchSize:           200,
		},
	}
}

func (c *Config) validate() error {
	if c.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if len(c.Auth.JWTSecret) < 16 {
		return fmt.Errorf("auth.jwt_secret must be at least 16 bytes (got %d)", len(c.Auth.JWTSecret))
	}
	if c.Auth.AccessTTLSeconds <= 0 {
		return fmt.Errorf("auth.access_ttl_seconds must be > 0")
	}
	if c.Auth.RefreshTTLSeconds <= 0 {
		return fmt.Errorf("auth.refresh_ttl_seconds must be > 0")
	}
	if c.Auth.BcryptCost < 4 || c.Auth.BcryptCost > 31 {
		return fmt.Errorf("auth.bcrypt_cost must be in [4, 31]")
	}
	if c.Telegram.BindTokenTTLSeconds < 0 {
		return fmt.Errorf("telegram.bind_token_ttl_seconds must be >= 0")
	}
	if c.Scheduler.TickIntervalSeconds < 0 {
		return fmt.Errorf("scheduler.tick_interval_seconds must be >= 0")
	}
	if c.Scheduler.BatchSize < 0 {
		return fmt.Errorf("scheduler.batch_size must be >= 0")
	}
	// telegram 启用时(填了 token)就要求填 bot_username 与 webhook_secret —— 否则 webhook 配置不上,deep-link 也拼不出。
	if c.Telegram.BotToken != "" {
		if c.Telegram.BotUsername == "" {
			return fmt.Errorf("telegram.bot_username is required when bot_token is set")
		}
		if c.Telegram.WebhookSecret == "" {
			return fmt.Errorf("telegram.webhook_secret is required when bot_token is set")
		}
	}
	return nil
}
