package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	log "github.com/sirupsen/logrus"
)

const (
	ContextEventIDKey          = "notification_event_id"
	ContextNotificationIDKey   = "notification_id"
	ContextNotificationNameKey = "notification_name"
)

type PushDeerConfig struct {
	PushKey  string `json:"push_key"`
	Endpoint string `json:"endpoint"`
	Type     string `json:"type"`
}

type AzureOAuthConfig struct {
	TenantID      string            `json:"tenant_id"`
	ClientID      string            `json:"client_id"`
	ClientSecret  string            `json:"client_secret"`
	Scope         string            `json:"scope"`
	TokenURL      string            `json:"token_url"`
	NotifyURL     string            `json:"notify_url"`
	Method        string            `json:"method"`
	Headers       map[string]string `json:"headers"`
	BodyTemplate  string            `json:"body_template"`
	TitleTemplate string            `json:"title_template"`
}

type azureTokenResp struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

func EventIDFromContext(ctx context.Context) string {
	eventID, _ := ctx.Value(ContextEventIDKey).(string)
	return eventID
}

func NotificationIDFromContext(ctx context.Context) uint {
	notificationID, _ := ctx.Value(ContextNotificationIDKey).(uint)
	return notificationID
}

func NotificationNameFromContext(ctx context.Context) string {
	notificationName, _ := ctx.Value(ContextNotificationNameKey).(string)
	return notificationName
}

func SendByID(ctx context.Context, id uint, title, body string) error {
	item, err := op.GetNotificationById(id)
	if err != nil {
		return err
	}
	return Send(ctx, item, title, body)
}

func Send(ctx context.Context, item *model.Notification, title, body string) error {
	if item == nil {
		return errors.New("notification not found")
	}
	if !item.Enabled {
		return errors.New("notification is disabled")
	}
	switch item.Type {
	case model.NotificationPushDeer:
		return sendPushDeer(ctx, item, title, body)
	case model.NotificationAzureOAuth:
		return sendAzureOAuth(ctx, item, title, body)
	default:
		return fmt.Errorf("unsupported notification type: %s", item.Type)
	}
}

func sendPushDeer(ctx context.Context, item *model.Notification, title, body string) error {
	var cfg PushDeerConfig
	config := strings.TrimSpace(item.Config)
	if config == "" {
		config = "{}"
	}
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return err
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api2.pushdeer.com/message/push"
	}
	if cfg.PushKey == "" {
		return errors.New("pushdeer push_key is empty")
	}
	form := url.Values{}
	form.Set("pushkey", cfg.PushKey)
	form.Set("text", title)
	form.Set("desp", body)
	if cfg.Type != "" {
		form.Set("type", cfg.Type)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return doNotifyRequest(req)
}

func sendAzureOAuth(ctx context.Context, item *model.Notification, title, body string) error {
	var cfg AzureOAuthConfig
	config := strings.TrimSpace(item.Config)
	if config == "" {
		config = "{}"
	}
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return err
	}
	if cfg.TokenURL == "" {
		if cfg.TenantID == "" {
			return errors.New("azure oauth tenant_id is empty")
		}
		cfg.TokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.TenantID)
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return errors.New("azure oauth client_id or client_secret is empty")
	}
	if cfg.NotifyURL == "" {
		return errors.New("azure oauth notify_url is empty")
	}
	token, err := getAzureOAuthToken(ctx, cfg)
	if err != nil {
		return err
	}
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}
	if cfg.TitleTemplate != "" {
		title = applyTemplate(cfg.TitleTemplate, title, body)
	}
	payload := applyTemplate(cfg.BodyTemplate, title, body)
	if payload == "" {
		defaultPayload, _ := json.Marshal(map[string]string{
			"title": title,
			"body":  body,
		})
		payload = string(defaultPayload)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.NotifyURL, bytes.NewBufferString(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AList-Notification-Title", title)
	for key, value := range cfg.Headers {
		req.Header.Set(key, applyTemplate(value, title, body))
	}
	return doNotifyRequest(req)
}

func getAzureOAuthToken(ctx context.Context, cfg AzureOAuthConfig) (string, error) {
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("grant_type", "client_credentials")
	if cfg.Scope != "" {
		form.Set("scope", cfg.Scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("azure oauth token request failed: %s", strings.TrimSpace(string(data)))
	}
	var token azureTokenResp
	if err = json.Unmarshal(data, &token); err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		return "", errors.New("azure oauth access_token is empty")
	}
	return token.AccessToken, nil
}

func doNotifyRequest(req *http.Request) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func applyTemplate(tpl, title, body string) string {
	if tpl == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"{{title}}", title,
		"{{body}}", body,
		"{{time}}", time.Now().Format(time.RFC3339),
	)
	return replacer.Replace(tpl)
}

type event struct {
	ID             string
	NotificationID uint
	Title          string
	Body           string
	StartedAt      time.Time
	mu             sync.Mutex
	pending        int
	failed         int
	errs           []string
	closed         bool
	done           bool
}

var (
	events   sync.Map
	eventSeq uint64
)

func NewEvent(notificationID uint, title, body string) (string, *model.Notification, error) {
	item, err := op.GetNotificationById(notificationID)
	if err != nil {
		return "", nil, err
	}
	if !item.Enabled {
		return "", nil, errors.New("notification is disabled")
	}
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), atomic.AddUint64(&eventSeq, 1))
	events.Store(id, &event{
		ID:             id,
		NotificationID: notificationID,
		Title:          title,
		Body:           body,
		StartedAt:      time.Now(),
	})
	return id, item, nil
}

func AddEventTask(eventID string) {
	if eventID == "" {
		return
	}
	raw, ok := events.Load(eventID)
	if !ok {
		return
	}
	evt := raw.(*event)
	evt.mu.Lock()
	defer evt.mu.Unlock()
	if !evt.done {
		evt.pending++
	}
}

func FinishEventTask(eventID string, taskErr error) {
	if eventID == "" {
		return
	}
	raw, ok := events.Load(eventID)
	if !ok {
		return
	}
	evt := raw.(*event)
	evt.mu.Lock()
	if evt.pending > 0 {
		evt.pending--
	}
	recordEventError(evt, taskErr)
	shouldDispatch := evt.pending == 0 && evt.closed && !evt.done
	if shouldDispatch {
		evt.done = true
	}
	evt.mu.Unlock()
	if shouldDispatch {
		dispatchEvent(evt)
	}
}

func FinishEventIfIdle(eventID string, taskErr error) {
	if eventID == "" {
		return
	}
	raw, ok := events.Load(eventID)
	if !ok {
		return
	}
	evt := raw.(*event)
	evt.mu.Lock()
	evt.closed = true
	recordEventError(evt, taskErr)
	shouldDispatch := evt.pending == 0 && !evt.done
	if shouldDispatch {
		evt.done = true
	}
	evt.mu.Unlock()
	if shouldDispatch {
		dispatchEvent(evt)
	}
}

func recordEventError(evt *event, taskErr error) {
	if taskErr == nil {
		return
	}
	evt.failed++
	if len(evt.errs) < 5 {
		evt.errs = append(evt.errs, taskErr.Error())
	}
}

func dispatchEvent(evt *event) {
	events.Delete(evt.ID)
	status := "success"
	if evt.failed > 0 {
		status = fmt.Sprintf("finished with %d failed task(s)", evt.failed)
	}
	body := fmt.Sprintf("%s\n\nStatus: %s\nStarted at: %s\nFinished at: %s",
		evt.Body,
		status,
		evt.StartedAt.Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
	)
	if len(evt.errs) > 0 {
		body += "\nErrors:\n- " + strings.Join(evt.errs, "\n- ")
	}
	go func() {
		if err := SendByID(context.Background(), evt.NotificationID, evt.Title, body); err != nil {
			log.Errorf("failed to send notification event %s: %+v", evt.ID, err)
		}
	}()
}
