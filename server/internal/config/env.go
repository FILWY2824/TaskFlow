package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadEnvFile 把 path(默认 ".env")里的 KEY=VALUE 行读到进程环境里。
//
// 设计要点:
//   - 已经存在的环境变量优先(进程环境 > .env 文件),与 docker-compose / dotenv 等
//     工具的常规约定一致。这样 systemd / k8s 注入的环境不会被本地 .env 覆盖。
//   - 不依赖第三方库(godotenv 等),自己解析 200 行内的简单格式即可。
//   - 文件不存在不算错误,直接返回 nil —— 大多数生产部署不会带 .env。
//
// 支持语法:
//   - `KEY=value`              字面量
//   - `KEY="value with spaces"` 双引号(支持转义 \" \\ \n)
//   - `KEY='raw value'`        单引号(原样,不解析转义)
//   - `# comment` / 空行       忽略
//   - `export KEY=value`       兼容 shell 风格的 export 前缀
//
// 不支持变量插值 ($FOO),保持简单。
func LoadEnvFile(path string) error {
	if path == "" {
		path = ".env"
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 4096), 1<<20) // 单行最多 1MB
	lineNo := 0
	for scan.Scan() {
		lineNo++
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return fmt.Errorf(".env line %d: missing '='", lineNo)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// 行尾注释:仅在没引号时去除。
		if !(strings.HasPrefix(val, "\"") || strings.HasPrefix(val, "'")) {
			if i := strings.Index(val, " #"); i >= 0 {
				val = strings.TrimSpace(val[:i])
			}
		}
		val, err = unquoteEnvValue(val)
		if err != nil {
			return fmt.Errorf(".env line %d: %w", lineNo, err)
		}
		// 进程环境优先 —— 已有的不覆盖。
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf(".env line %d: setenv %s: %w", lineNo, key, err)
		}
	}
	return scan.Err()
}

func unquoteEnvValue(v string) (string, error) {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		// 双引号:解析最常见的转义 \" \\ \n \r \t
		inner := v[1 : len(v)-1]
		var b strings.Builder
		for i := 0; i < len(inner); i++ {
			c := inner[i]
			if c == '\\' && i+1 < len(inner) {
				switch inner[i+1] {
				case '"':
					b.WriteByte('"')
				case '\\':
					b.WriteByte('\\')
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(inner[i+1])
				}
				i++
				continue
			}
			b.WriteByte(c)
		}
		return b.String(), nil
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		// 单引号:原样
		return v[1 : len(v)-1], nil
	}
	return v, nil
}

// LoadOAuthFromEnv 把 OAUTH_* 环境变量读进 OAuthConfig 并做基本校验。
//
// 变量列表见 .env.example。Enabled = true 时核心字段(URLs / client_id /
// client_secret / redirect_url)必须齐全,否则 Load 会报错并阻止启动。
func LoadOAuthFromEnv() (OAuthConfig, error) {
	cfg := OAuthConfig{
		Enabled:             envBool("OAUTH_ENABLED"),
		Provider:            os.Getenv("OAUTH_PROVIDER"),
		AuthorizeURL:        os.Getenv("OAUTH_AUTHORIZE_URL"),
		TokenURL:            os.Getenv("OAUTH_TOKEN_URL"),
		UserInfoURL:         os.Getenv("OAUTH_USERINFO_URL"),
		ClientID:            os.Getenv("OAUTH_CLIENT_ID"),
		ClientSecret:        os.Getenv("OAUTH_CLIENT_SECRET"),
		RedirectURL:         os.Getenv("OAUTH_REDIRECT_URL"),
		FrontendRedirectURL: os.Getenv("OAUTH_FRONTEND_REDIRECT_URL"),
		EmailField:          os.Getenv("OAUTH_EMAIL_FIELD"),
		NameField:           os.Getenv("OAUTH_NAME_FIELD"),
		SubjectField:        os.Getenv("OAUTH_SUBJECT_FIELD"),
		Scopes:              splitScopes(os.Getenv("OAUTH_SCOPES")),
	}
	if !cfg.Enabled {
		return cfg, nil
	}
	missing := []string{}
	check := func(name, v string) {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	check("OAUTH_PROVIDER", cfg.Provider)
	check("OAUTH_AUTHORIZE_URL", cfg.AuthorizeURL)
	check("OAUTH_TOKEN_URL", cfg.TokenURL)
	check("OAUTH_USERINFO_URL", cfg.UserInfoURL)
	check("OAUTH_CLIENT_ID", cfg.ClientID)
	check("OAUTH_CLIENT_SECRET", cfg.ClientSecret)
	check("OAUTH_REDIRECT_URL", cfg.RedirectURL)
	if len(missing) > 0 {
		return cfg, fmt.Errorf("OAUTH_ENABLED=true but missing env vars: %v", missing)
	}
	if cfg.EmailField == "" {
		cfg.EmailField = "email"
	}
	if cfg.NameField == "" {
		cfg.NameField = "name"
	}
	if cfg.SubjectField == "" {
		cfg.SubjectField = "sub"
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email"}
	}
	return cfg, nil
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on", "y", "t":
		return true
	}
	return false
}

func splitScopes(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	// 支持空格 / 逗号分隔(OAuth 规范是空格,但用户常写逗号)。
	repl := strings.NewReplacer(",", " ", "\t", " ")
	v = repl.Replace(v)
	out := []string{}
	for _, s := range strings.Fields(v) {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
