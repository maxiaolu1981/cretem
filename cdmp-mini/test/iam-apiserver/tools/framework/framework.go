package framework

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/ratelimiter"
	"golang.org/x/time/rate"
)

type APIResponse struct {
	Code       int             `json:"code"`
	Message    string          `json:"message"`
	Error      string          `json:"error"`
	Data       json.RawMessage `json:"data"`
	httpStatus int
}

func (r *APIResponse) HTTPStatus() int {
	if r == nil {
		return 0
	}
	return r.httpStatus
}

type AuthTokens struct {
	Username     string
	UserID       string
	AccessToken  string
	RefreshToken string
}

type LoginOptions struct {
	Headers map[string]string
}

type AuditEvent struct {
	Actor        string         `json:"Actor"`
	ActorID      string         `json:"ActorID"`
	Action       string         `json:"Action"`
	ResourceType string         `json:"ResourceType"`
	ResourceID   string         `json:"ResourceID"`
	Target       string         `json:"Target"`
	Outcome      string         `json:"Outcome"`
	ErrorMessage string         `json:"ErrorMessage"`
	RequestID    string         `json:"RequestID"`
	IP           string         `json:"IP"`
	UserAgent    string         `json:"UserAgent"`
	Metadata     map[string]any `json:"Metadata"`
	OccurredAt   time.Time      `json:"OccurredAt"`
}

type Env struct {
	BaseURL         string
	AdminUsername   string
	AdminPassword   string
	AdminToken      string
	Client          *http.Client
	OutputRoot      string
	random          *rand.Rand
	adminTokenOnce  sync.Once
	adminTokenErr   error
	limiters        map[string]*rate.Limiter
	defaultLimiter  *rate.Limiter
	producerLimiter *ratelimiter.RateLimiterController
	rateLimiterInfo rateLimiterSnapshot
	rateLimiterOnce sync.Once
	lazyAdminLogin  bool
}

const (
	defaultBaseURL   = "http://192.168.10.8:8088"
	defaultAdminUser = "admin"
	defaultAdminPass = "Admin@2021"
	requestTimeout   = 30 * time.Second
)

func NewEnv(t *testing.T) *Env {
	t.Helper() // 标记此函数为辅助函数

	if os.Getenv("IAM_APISERVER_E2E") == "" {
		t.Fatalf("login before change failed: %v", errors.New("IAM_APISERVER_E2E not set"))
	}

	baseURL := os.Getenv("IAM_APISERVER_BASEURL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	adminUser := os.Getenv("IAM_APISERVER_ADMIN_USER")
	if adminUser == "" {
		adminUser = defaultAdminUser
	}
	adminPass := os.Getenv("IAM_APISERVER_ADMIN_PASS")
	if adminPass == "" {
		adminPass = defaultAdminPass
	}

	env := &Env{
		BaseURL:       strings.TrimRight(baseURL, "/"),
		AdminUsername: adminUser,
		AdminPassword: adminPass,
		Client:        &http.Client{Timeout: requestTimeout},
		OutputRoot:    "output",
		random:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if flag := strings.TrimSpace(os.Getenv("IAM_APISERVER_LAZY_ADMIN_LOGIN")); flag != "" {
		switch strings.ToLower(flag) {
		case "0", "false", "no", "off":
			env.lazyAdminLogin = false
		default:
			env.lazyAdminLogin = true
		}
	} else {
		env.lazyAdminLogin = true
	}

	opts := options.NewServerRunOptions()
	opts.Complete()
	kafkaOpts := options.NewKafkaOptions()
	kafkaOpts.Complete()
	applyKafkaOverrides(kafkaOpts)
	env.rateLimiterInfo = rateLimiterSnapshot{
		Enabled:      opts.EnableRateLimiter,
		StartingRate: float64(kafkaOpts.StartingRate),
		MinRate:      float64(kafkaOpts.MinRate),
		MaxRate:      float64(kafkaOpts.MaxRate),
		AdjustPeriod: kafkaOpts.AdjustPeriod.String(),
	}
	if opts.EnableRateLimiter {
		env.limiters = make(map[string]*rate.Limiter)
		if loginLimiter := newRateLimiter(opts.LoginRateLimit, opts.LoginWindow); loginLimiter != nil {
			env.limiters["login"] = loginLimiter
		}
		if writeLimiter := newRateLimiter(opts.WriteRateLimit, time.Minute); writeLimiter != nil {
			env.limiters["write"] = writeLimiter
			env.defaultLimiter = writeLimiter
		}
		env.initProducerLimiter(t, kafkaOpts)
	}
	if !opts.EnableRateLimiter {
		env.rateLimiterInfo.StatsSource = ""
	}

	t.Cleanup(func() {
		if env.producerLimiter != nil {
			env.producerLimiter.Stop()
		}
	})

	env.ensureOutputRoot(t)
	if !env.lazyAdminLogin {
		env.ensureAdminToken(t)
	}
	return env
}

func (e *Env) ensureOutputRoot(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(e.OutputRoot, 0o755); err != nil {
		t.Fatalf("login before change failed: %v", fmt.Errorf("create output root: %w", err))
	}
}

func (e *Env) ensureAdminToken(t *testing.T) {
	t.Helper()
	if err := e.ensureAdminTokenInternal(); err != nil {
		t.Fatalf("login before change failed: %v", err)
	}
}

func (e *Env) ensureAdminTokenInternal() error {
	e.adminTokenOnce.Do(func() {
		tok, _, err := e.Login(e.AdminUsername, e.AdminPassword)
		if err != nil {
			e.adminTokenErr = fmt.Errorf("admin login: %w", err)
			return
		}
		if tok == nil || tok.AccessToken == "" {
			e.adminTokenErr = fmt.Errorf("admin login returned empty tokens")
			return
		}
		e.AdminToken = tok.AccessToken
	})
	return e.adminTokenErr
}

func (e *Env) AdminTokenOrFail(t *testing.T) string {
	t.Helper()
	if err := e.ensureAdminTokenInternal(); err != nil {
		t.Fatalf("login before change failed: %v", err)
	}
	return e.AdminToken
}

func (e *Env) newRequest(method, path string, body []byte) (*http.Request, error) {
	url := e.BaseURL + path
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (e *Env) do(req *http.Request) (*APIResponse, error) {
	e.waitRateLimit(req.Method, req.URL.Path)
	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var apiResp APIResponse
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &apiResp); err != nil {
			return nil, fmt.Errorf("decode api response: %w: %s", err, string(raw))
		}
	}
	apiResp.httpStatus = resp.StatusCode
	return &apiResp, nil
}

func (e *Env) Login(username, password string) (*AuthTokens, *APIResponse, error) {
	return e.LoginWithOptions(username, password, nil)
}

func (e *Env) LoginWithOptions(username, password string, opts *LoginOptions) (*AuthTokens, *APIResponse, error) {
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	req, err := e.newRequest(http.MethodPost, "/login", body)
	if err != nil {
		return nil, nil, err
	}
	if opts != nil && len(opts.Headers) > 0 {
		for k, v := range opts.Headers {
			req.Header.Set(k, v)
		}
	}
	apiResp, err := e.do(req)
	if err != nil {
		return nil, nil, err
	}
	if apiResp.httpStatus != http.StatusOK {
		return nil, apiResp, fmt.Errorf("unexpected status: %d", apiResp.httpStatus)
	}
	var data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		UserID       string `json:"user_id"`
	}
	if len(apiResp.Data) > 0 {
		if err := json.Unmarshal(apiResp.Data, &data); err != nil {
			return nil, apiResp, fmt.Errorf("decode login data: %w", err)
		}
	}
	return &AuthTokens{Username: username, AccessToken: data.AccessToken, RefreshToken: data.RefreshToken, UserID: data.UserID}, apiResp, nil
}

func (e *Env) AuditEvents(limit int) ([]AuditEvent, bool, *APIResponse, error) {
	if limit < 0 {
		limit = 0
	}
	path := "/admin/audit/events"
	if limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, limit)
	}
	resp, err := e.AdminRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, false, resp, err
	}
	if resp == nil {
		return nil, false, nil, fmt.Errorf("nil response fetching audit events")
	}
	if resp.HTTPStatus() != http.StatusOK {
		return nil, false, resp, fmt.Errorf("unexpected status: %d", resp.HTTPStatus())
	}
	var payload struct {
		Events  []AuditEvent `json:"events"`
		Enabled bool         `json:"enabled"`
	}
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &payload); err != nil {
			return nil, false, resp, fmt.Errorf("decode audit events: %w", err)
		}
	}
	return payload.Events, payload.Enabled, resp, nil
}

func (e *Env) SetLoginRateLimit(value int) (*APIResponse, error) {
	payload := map[string]int{"value": value}
	return e.AdminRequest(http.MethodPost, "/admin/ratelimit/login", payload)
}

func (e *Env) ResetLoginRateLimit() (*APIResponse, error) {
	return e.AdminRequest(http.MethodDelete, "/admin/ratelimit/login", nil)
}

func (e *Env) LoginRateLimiterEnabled() bool {
	return e.rateLimiterInfo.Enabled
}

func (e *Env) LoginOrFail(t *testing.T, username, password string) *AuthTokens {
	t.Helper()
	tokens, _, err := e.Login(username, password)
	if err != nil {
		t.Fatalf("login before change failed: %v", fmt.Errorf("login %s: %w", username, err))
	}
	if tokens == nil || tokens.AccessToken == "" {
		t.Fatalf("login before change failed: %v", fmt.Errorf("login %s returned empty tokens", username))
	}
	return tokens
}

func (e *Env) AuthorizedRequest(method, path, token string, payload any) (*APIResponse, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	req, err := e.newRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return e.do(req)
}

func (e *Env) AdminRequest(method, path string, payload any) (*APIResponse, error) {
	token, err := e.adminTokenValue()
	if err != nil {
		return nil, err
	}
	return e.AuthorizedRequest(method, path, token, payload)
}

func (e *Env) adminTokenValue() (string, error) {
	if err := e.ensureAdminTokenInternal(); err != nil {
		return "", err
	}
	return e.AdminToken, nil
}

func (e *Env) RandomUsername(prefix string) string {
	return fmt.Sprintf("%s%d_%d", prefix, time.Now().UnixNano(), e.random.Intn(100000))
}

type UserSpec struct {
	Name     string
	Nickname string
	Password string
	Email    string
	Phone    string
	Status   int
	IsAdmin  int
}

func (e *Env) NewUserSpec(prefix, password string) UserSpec {
	username := e.RandomUsername(prefix)
	return UserSpec{
		Name:     username,
		Nickname: "集成测试用户",
		Password: password,
		Email:    fmt.Sprintf("%s@example.com", username),
		Phone:    fmt.Sprintf("138%08d", e.random.Intn(100000000)),
		Status:   1,
		IsAdmin:  0,
	}
}

func (e *Env) CreateUser(spec UserSpec) (*APIResponse, error) {
	payload := map[string]any{
		"metadata": map[string]string{"name": spec.Name},
		"nickname": spec.Nickname,
		"password": spec.Password,
		"email":    spec.Email,
		"phone":    spec.Phone,
		"status":   spec.Status,
		"isAdmin":  spec.IsAdmin,
	}
	//	fmt.Printf("Creating user: %+v\n", payload)
	return e.AdminRequest(http.MethodPost, "/v1/users", payload)
}

func (e *Env) CreateUserAndWait(t *testing.T, spec UserSpec, wait time.Duration) {
	t.Helper()
	resp, err := e.CreateUser(spec)
	if err != nil {
		t.Fatalf("login before change failed: %v", fmt.Errorf("create user %s: %w", spec.Name, err))
	}
	if resp.HTTPStatus() != http.StatusCreated {
		t.Fatalf("login before change failed: %v", fmt.Errorf("create user http=%d code=%d", resp.HTTPStatus(), resp.Code))
	}
	if wait <= 0 {
		wait = 5 * time.Second
	}
	if err := e.WaitForUser(spec.Name, wait); err != nil {
		t.Fatalf("login before change failed: %v", fmt.Errorf("wait for user %s: %w", spec.Name, err))
	}
}

func (e *Env) ForceDeleteUser(username string) (*APIResponse, error) {
	path := fmt.Sprintf("/v1/users/%s/force", username)
	req, err := e.newRequest(http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	token, err := e.adminTokenValue()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return e.do(req)
}

func (e *Env) ForceDeleteUserIgnore(username string) {
	if username == "" {
		return
	}
	if _, err := e.ForceDeleteUser(username); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] cleanup user %s failed: %v\n", username, err)
	}
}

func (e *Env) GetUser(token, username string) (*APIResponse, error) {
	path := fmt.Sprintf("/v1/users/%s", username)
	return e.AuthorizedRequest(http.MethodGet, path, token, nil)
}

func (e *Env) UpdateUser(spec UserSpec) (*APIResponse, error) {
	payload := map[string]any{
		"metadata": map[string]string{"name": spec.Name},
		"nickname": spec.Nickname,
		"email":    spec.Email,
		"phone":    spec.Phone,
		"status":   spec.Status,
		"isAdmin":  spec.IsAdmin,
	}
	path := fmt.Sprintf("/v1/users/%s", spec.Name)
	return e.AdminRequest(http.MethodPut, path, payload)
}

func (e *Env) ChangePassword(token, username, oldPassword, newPassword string) (*APIResponse, error) {
	path := fmt.Sprintf("/v1/users/%s/change-password", username)
	payload := map[string]string{
		"oldPassword": oldPassword,
		"newPassword": newPassword,
	}
	return e.AuthorizedRequest(http.MethodPut, path, token, payload)
}

func (e *Env) Logout(token, refreshToken string) (*APIResponse, error) {
	payload := map[string]string{"refresh_token": refreshToken}
	return e.AuthorizedRequest(http.MethodPost, "/logout", token, payload)
}

func (e *Env) Refresh(token, refreshToken string) (*APIResponse, error) {
	payload := map[string]string{"refresh_token": refreshToken}
	return e.AuthorizedRequest(http.MethodPost, "/refresh", token, payload)
}

func (e *Env) ListUsers(token string) (*APIResponse, error) {
	return e.AuthorizedRequest(http.MethodGet, "/v1/users", token, nil)
}

func (e *Env) EnsureOutputDir(t *testing.T, testDir string) string {
	t.Helper()
	full := filepath.Join(testDir, e.OutputRoot)

	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("login before change failed: %v", fmt.Errorf("create output dir: %w", err))
	}
	e.writeRateLimiterSnapshot(t, full)
	return full
}

func (e *Env) WaitForUser(username string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// 首次等待稍长时间，避免立即检查
	time.Sleep(500 * time.Millisecond)

	for {
		resp, err := e.AdminRequest(http.MethodGet, fmt.Sprintf("/v1/users/%s", username), nil)
		if err == nil && resp != nil && resp.httpStatus == http.StatusOK {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("user %s not ready", username)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (e *Env) waitRateLimit(method, path string) {
	var limiter *rate.Limiter
	if strings.EqualFold(path, "/login") {
		limiter = e.limiters["login"]
	} else if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete {
		limiter = e.limiters["write"]
	}
	if limiter == nil {
		limiter = e.defaultLimiter
	}
	if limiter == nil {
		goto producerLimiter
	}
	_ = limiter.Wait(context.Background())

producerLimiter:
	if e.producerLimiter == nil {
		return
	}
	_ = e.producerLimiter.Wait(context.Background())
}

func newRateLimiter(limit int, window time.Duration) *rate.Limiter {
	if limit <= 0 {
		return nil
	}
	if window <= 0 {
		window = time.Second
	}
	ratePerSecond := float64(limit) / window.Seconds()
	if ratePerSecond <= 0 {
		return nil
	}
	burst := limit
	if burst <= 0 {
		burst = 1
	}
	return rate.NewLimiter(rate.Limit(ratePerSecond), burst)
}

type rateLimiterSnapshot struct {
	Enabled      bool    `json:"enabled"`
	StartingRate float64 `json:"starting_rate"`
	MinRate      float64 `json:"min_rate"`
	MaxRate      float64 `json:"max_rate"`
	AdjustPeriod string  `json:"adjust_period"`
	StatsSource  string  `json:"stats_source,omitempty"`
}

func (e *Env) initProducerLimiter(t *testing.T, kafkaOpts *options.KafkaOptions) {
	statsURL := fmt.Sprintf("%s/metrics", e.BaseURL)
	statsProvider := e.newProducerStatsProvider(statsURL)
	e.producerLimiter = ratelimiter.NewRateLimiterController(
		float64(kafkaOpts.StartingRate),
		float64(kafkaOpts.MinRate),
		float64(kafkaOpts.MaxRate),
		kafkaOpts.AdjustPeriod,
		statsProvider,
	)
	e.rateLimiterInfo.StatsSource = statsURL
}

func (e *Env) writeRateLimiterSnapshot(t *testing.T, outputDir string) {
	e.rateLimiterOnce.Do(func() {
		snapshot := e.rateLimiterInfo
		if !snapshot.Enabled {
			snapshot.StatsSource = ""
		}
		raw, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			t.Fatalf("login before change failed: %v", fmt.Errorf("marshal rate limiter snapshot: %w", err))
		}
		path := filepath.Join(outputDir, "rate_limiter_snapshot.json")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatalf("login before change failed: %v", fmt.Errorf("write rate limiter snapshot: %w", err))
		}
	})
}

func applyKafkaOverrides(kafkaOpts *options.KafkaOptions) {
	if v := os.Getenv("IAM_APISERVER_KAFKA_STARTING_RATE"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			kafkaOpts.StartingRate = parsed
		}
	}
	if v := os.Getenv("IAM_APISERVER_KAFKA_MIN_RATE"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			kafkaOpts.MinRate = parsed
		}
	}
	if v := os.Getenv("IAM_APISERVER_KAFKA_MAX_RATE"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			kafkaOpts.MaxRate = parsed
		}
	}
	if v := os.Getenv("IAM_APISERVER_KAFKA_ADJUST_PERIOD"); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			kafkaOpts.AdjustPeriod = parsed
		}
	}
}

func (e *Env) newProducerStatsProvider(metricsURL string) func() (int, int) {
	client := &http.Client{Timeout: 5 * time.Second}
	return func() (int, int) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
		if err != nil {
			return 0, 0
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, 0
		}
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		total := 0.0
		fail := 0.0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "kafka_producer_success_total") {
				total += parseMetricValue(line)
			} else if strings.HasPrefix(line, "kafka_producer_failures_total") {
				value := parseMetricValue(line)
				fail += value
				total += value
			}
		}
		return int(total), int(fail)
	}
}

func parseMetricValue(line string) float64 {
	idx := strings.LastIndex(line, " ")
	if idx == -1 {
		return 0
	}
	valueStr := strings.TrimSpace(line[idx+1:])
	if valueStr == "NaN" || valueStr == "+Inf" {
		return 0
	}
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return 0
	}
	return value
}
