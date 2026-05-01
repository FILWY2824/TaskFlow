package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/youruser/taskflow/internal/auth"
)

type ctxKey int

const (
	ctxKeyUserID ctxKey = iota
)

// UserIDFrom 从 context 取当前用户 id;未认证时返回 0。
func UserIDFrom(ctx context.Context) int64 {
	v, _ := ctx.Value(ctxKeyUserID).(int64)
	return v
}

// WithUserID 把 userID 写到 context(主要用于测试)。
func WithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, uid)
}

// RequireAuth 要求请求带有合法 Bearer token,否则 401。
func RequireAuth(issuer *auth.Issuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				writeAuthError(w, "missing bearer token")
				return
			}
			tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
			if tok == "" {
				writeAuthError(w, "empty token")
				return
			}
			claims, err := issuer.ParseAccessToken(tok)
			if err != nil {
				if errors.Is(err, auth.ErrTokenExpired) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":{"code":"token_expired","message":"access token expired"}}`))
					return
				}
				writeAuthError(w, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyUserID, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminChecker 用来在 RequireAdmin 中查询当前用户是否管理员。
// 之所以抽象成接口而不直接拿 *store.UserStore,是为了让测试可以用桩对象,
// 同时避免 middleware 包反向依赖 store 包。
type AdminChecker interface {
	IsAdmin(ctx context.Context, userID int64) (bool, error)
}

// RequireAdmin 在 RequireAuth 之后再加一道:当前用户必须是管理员才放行。
// 不是管理员 -> 403 forbidden。查询 DB 失败 -> 503,避免在数据库故障时把所有请求当成管理员。
func RequireAdmin(checker AdminChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid := UserIDFrom(r.Context())
			if uid == 0 {
				writeAuthError(w, "login required")
				return
			}
			ok, err := checker.IsAdmin(r.Context(), uid)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":"db_unavailable","message":"failed to verify admin"}}`))
				return
			}
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"admin only"}}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeAuthError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"` + msg + `"}}`))
}

// Logger 简单记录每个请求耗时和状态码。
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rw, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", clientIP(r),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Recover 拦截 panic,防止整个进程崩溃。
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "err", rec, "stack", string(debug.Stack()))
					http.Error(w, `{"error":{"code":"internal","message":"internal server error"}}`,
						http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS 简单 CORS。生产环境若 Web 端与 API 同域(经 Nginx 反代),其实可以不加。
// 这里默认允许所有来源,POST/PUT/DELETE/GET 方法和常用 header。
func CORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Device-Id")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Chain 把多个 middleware 按从外到内的顺序依次包裹。
// Chain(A, B, C)(h) == A(B(C(h)))
func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			h = mws[i](h)
		}
		return h
	}
}

// ClientIP 返回最佳猜测的来源 IP(可被反向代理覆盖)。供审计日志记录使用。
func ClientIP(r *http.Request) string { return clientIP(r) }

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.Index(v, ","); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	return r.RemoteAddr
}
