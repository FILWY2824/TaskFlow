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

// PublicBaseURL 返回去掉末尾斜杠的 PUBLIC_BASE_URL(前端域名),没设置则空串。
func PublicBaseURL() string {
	v := strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL"))
	return strings.TrimRight(v, "/")
}

// PublicApiURL 返回去掉末尾斜杠的 PUBLIC_API_URL(后端 API 域名),没设置则
// 回退到 PUBLIC_BASE_URL(单域名部署兼容)。
func PublicApiURL() string {
	v := strings.TrimSpace(os.Getenv("PUBLIC_API_URL"))
	if v == "" {
		v = os.Getenv("PUBLIC_BASE_URL")
	}
	return strings.TrimRight(v, "/")
}

// LoadOAuthFromEnv 把 OAUTH_* 环境变量读进 OAuthConfig 并做基本校验。
//
// 变量列表见 .env.example。Enabled = true 时核心字段(URLs / client_id /
// client_secret / redirect_url)必须齐全,否则 Load 会报错并阻止启动。
//
// 当 PUBLIC_BASE_URL 设置而 OAUTH_REDIRECT_URL / OAUTH_FRONTEND_REDIRECT_URL
// 未设置时,会自动按 ${PUBLIC_BASE_URL}/api/auth/oauth/callback 与
// ${PUBLIC_BASE_URL}/oauth/callback 推导,免去用户重复填写。
//
// 另外做一些常见的 typo 兜底:
//   - URL 出现 "https://https://" / "http://http://" 直接报错(用户配错时立刻显形)
//   - URL 缺少 scheme(纯 host)时报错
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

	// 重定向 URL 推导(前后端分离):
	//   - RedirectURL(OAuth 回调)       → ${PUBLIC_API_URL}/api/auth/oauth/callback
	//   - FrontendRedirectURL(登录成功页) → ${PUBLIC_BASE_URL}/oauth/callback
	// PUBLIC_API_URL 没填时回退到 PUBLIC_BASE_URL(单域名部署兼容)。
	apiBase := PublicApiURL()
	frontBase := PublicBaseURL()
	if apiBase != "" && strings.TrimSpace(cfg.RedirectURL) == "" {
		cfg.RedirectURL = apiBase + "/api/auth/oauth/callback"
	}
	if frontBase != "" && strings.TrimSpace(cfg.FrontendRedirectURL) == "" {
		cfg.FrontendRedirectURL = frontBase + "/oauth/callback"
	}

	// typo 兜底:这是用户最容易踩的坑(双 https://、忘写协议头)。
	for _, p := range []struct {
		name string
		val  string
	}{
		{"OAUTH_AUTHORIZE_URL", cfg.AuthorizeURL},
		{"OAUTH_TOKEN_URL", cfg.TokenURL},
		{"OAUTH_USERINFO_URL", cfg.UserInfoURL},
		{"OAUTH_REDIRECT_URL", cfg.RedirectURL},
		{"OAUTH_FRONTEND_REDIRECT_URL", cfg.FrontendRedirectURL},
		{"PUBLIC_BASE_URL", PublicBaseURL()},
	} {
		if err := validateURLEnv(p.name, p.val); err != nil {
			return cfg, err
		}
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
		return cfg, fmt.Errorf("OAUTH_ENABLED=true but missing env vars: %v "+
			"(提示:可以只填 PUBLIC_BASE_URL,OAUTH_REDIRECT_URL / OAUTH_FRONTEND_REDIRECT_URL 会自动推导)", missing)
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

// validateURLEnv 给一个 *_URL 类型的环境变量做最常见的 typo 兜底。
//
// 用户最容易写出的两种错误:
//  1. 双 scheme:  https://https://example.com   (粘贴时多复制了一份)
//  2. 漏 scheme:  example.com                   (后端自己 302 时会失败)
//
// 这两种我们直接报错并指出原因,胜过半年后用户在生产里看 "redirect 失败" 才发现。
// 空字符串不在这里报错 —— 调用方有自己的"必填"检查。
func validateURLEnv(name, v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	low := strings.ToLower(v)
	if strings.Contains(low, "https://https://") ||
		strings.Contains(low, "http://http://") ||
		strings.Contains(low, "https://http://") ||
		strings.Contains(low, "http://https://") {
		return fmt.Errorf("%s: 检测到重复的协议头 %q —— 请检查是否粘贴时多带了 'https://'", name, v)
	}
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return fmt.Errorf("%s: 必须以 http:// 或 https:// 开头,实际值 %q", name, v)
	}
	return nil
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
