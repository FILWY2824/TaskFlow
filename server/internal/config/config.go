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
func Load(path string) (*Config, error) {
	cfg := defaults()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	// 确保数据库目录存在
	if dir := filepath.Dir(cfg.Database.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return cfg, nil
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
