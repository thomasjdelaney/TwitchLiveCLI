package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	twitchOAuth        = "https://id.twitch.tv/oauth2"
	scopeFollows       = "user:read:follows"
	scopeSubscriptions = "user:read:subscriptions"
	keyringService     = "live-followed-cli"
	keyringAccount     = "twitch-oauth"
)

type Credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type credentialStore interface {
	Load() (*Credentials, error)
	Save(Credentials) error
	Delete() error
}

type CredentialStore struct{ Disabled bool }

func (s *CredentialStore) Load() (*Credentials, error) {
	if s.Disabled {
		return nil, nil
	}
	value, err := keyring.Get(keyringService, keyringAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("platform credential store unavailable: %w (install or unlock Keychain, Windows Credential Manager, or a Secret Service on Linux; alternatively use --no-store for this run)", err)
	}
	var c Credentials
	if err := json.Unmarshal([]byte(value), &c); err != nil {
		return nil, errors.New("saved credentials could not be read; run with --logout after restoring the credential store")
	}
	if c.AccessToken == "" {
		return nil, errors.New("saved access token is empty; run --logout and authenticate again")
	}
	return &c, nil
}
func (s *CredentialStore) Save(c Credentials) error {
	if s.Disabled {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringService, keyringAccount, string(b)); err != nil {
		return fmt.Errorf("save credentials to platform store: %w", err)
	}
	return nil
}
func (s *CredentialStore) Delete() error {
	if err := keyring.Delete(keyringService, keyringAccount); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

type OAuth struct {
	ClientID              string
	Store                 credentialStore
	Subscriptions         bool
	HTTP                  *http.Client
	BaseURL               string
	OnDeviceAuthorization func(verificationURL, userCode string)
	credentials           *Credentials
	validatedAt           time.Time
	userID                string
}

func (o *OAuth) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}
func (o *OAuth) baseURL() string {
	if o.BaseURL != "" {
		return strings.TrimRight(o.BaseURL, "/")
	}
	return twitchOAuth
}
func (o *OAuth) Token(ctx context.Context) (string, error) {
	if o.credentials == nil {
		c, err := o.Store.Load()
		if err != nil {
			return "", err
		}
		o.credentials = c
	}
	if o.credentials == nil {
		c, err := o.deviceLogin(ctx)
		if err != nil {
			return "", err
		}
		o.credentials = &c
	}
	if time.Since(o.validatedAt) >= time.Hour {
		if err := o.validateOrRenew(ctx); err != nil {
			return "", err
		}
	}
	return o.credentials.AccessToken, nil
}

func (o *OAuth) validateOrRenew(ctx context.Context) error {
	err := o.validate(ctx)
	if err == nil {
		return nil
	}
	if errors.Is(err, errMissingSubscriptionScope) {
		return o.upgradeSubscriptionScope(ctx)
	}
	if !errors.Is(err, errInvalidToken) {
		return err
	}
	if err := o.refreshOrLogin(ctx); err != nil {
		if !errors.Is(err, errMissingSubscriptionScope) {
			return err
		}
		return o.upgradeSubscriptionScope(ctx)
	}
	return nil
}

var errInvalidToken = errors.New("invalid Twitch access token")
var errRefreshRejected = errors.New("Twitch rejected the refresh token")
var errMissingSubscriptionScope = errors.New("Twitch token is missing user:read:subscriptions")

type oauthResponseError struct {
	Status  int
	Message string
}

func (e *oauthResponseError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message) }

func (o *OAuth) validate(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL()+"/validate", nil)
	if err != nil {
		return fmt.Errorf("create Twitch token validation request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+o.credentials.AccessToken)
	resp, err := o.client().Do(req)
	if err != nil {
		return fmt.Errorf("validate Twitch token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errInvalidToken
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Twitch token validation returned HTTP %d", resp.StatusCode)
	}
	var v struct {
		ClientID string   `json:"client_id"`
		Scopes   []string `json:"scopes"`
		UserID   string   `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return fmt.Errorf("decode token validation response: %w", err)
	}
	if v.ClientID != o.ClientID {
		return errors.New("saved token belongs to a different Twitch client ID; run --logout and authenticate again")
	}
	if v.UserID == "" {
		return errors.New("Twitch returned a token without a user identity")
	}
	if !has(v.Scopes, scopeFollows) {
		return errors.New("Twitch token is missing user:read:follows; run --logout and authorize again")
	}
	if o.Subscriptions && !has(v.Scopes, scopeSubscriptions) {
		return errMissingSubscriptionScope
	}
	o.userID = v.UserID
	o.validatedAt = time.Now()
	return nil
}

func (o *OAuth) deviceLogin(ctx context.Context) (Credentials, error) {
	credentials, err := o.deviceAuthorization(ctx, o.requestedScopes())
	if err != nil {
		return Credentials{}, err
	}
	if err := o.Store.Save(credentials); err != nil {
		return Credentials{}, err
	}
	return credentials, nil
}

func (o *OAuth) requestedScopes() []string {
	scopes := []string{scopeFollows}
	if o.Subscriptions {
		scopes = append(scopes, scopeSubscriptions)
	}
	return scopes
}

func (o *OAuth) deviceAuthorization(ctx context.Context, scopes []string) (Credentials, error) {
	scopeValue := strings.Join(scopes, " ")
	form := url.Values{"client_id": {o.ClientID}, "scopes": {scopeValue}}
	var d struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := o.postForm(ctx, o.baseURL()+"/device", form, &d); err != nil {
		return Credentials{}, fmt.Errorf("start Twitch device authorization: %w", err)
	}
	if d.Interval < 1 {
		d.Interval = 5
	}
	if o.OnDeviceAuthorization != nil {
		o.OnDeviceAuthorization(d.VerificationURI, d.UserCode)
	} else {
		fmt.Printf("\nSign in to Twitch to continue:\n  %s\n  Enter code: %s\n\nWaiting for authorization...\n", d.VerificationURI, d.UserCode)
	}
	_ = openBrowser(d.VerificationURI)
	deadline := time.Now().Add(time.Duration(d.ExpiresIn) * time.Second)
	for {
		wait := time.Duration(d.Interval) * time.Second
		if time.Until(deadline) <= 0 {
			return Credentials{}, errors.New("Twitch device code expired; run the command again")
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Credentials{}, ctx.Err()
		case <-timer.C:
		}
		form := url.Values{"client_id": {o.ClientID}, "scopes": {scopeValue}, "device_code": {d.DeviceCode}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}}
		var c CredentialsWithExpiry
		err := o.postForm(ctx, o.baseURL()+"/token", form, &c)
		if err == nil {
			if c.AccessToken == "" || c.RefreshToken == "" {
				return Credentials{}, errors.New("Twitch returned an incomplete token response")
			}
			return Credentials{AccessToken: c.AccessToken, RefreshToken: c.RefreshToken}, nil
		}
		if strings.Contains(err.Error(), "authorization_pending") {
			continue
		}
		if strings.Contains(err.Error(), "slow_down") {
			d.Interval += 5
			continue
		}
		return Credentials{}, fmt.Errorf("complete Twitch device authorization: %w", err)
	}
}

func (o *OAuth) upgradeSubscriptionScope(ctx context.Context) error {
	previousCredentials := o.credentials
	previousUserID := o.userID
	previousValidatedAt := o.validatedAt
	credentials, err := o.deviceAuthorization(ctx, o.requestedScopes())
	if err != nil {
		return fmt.Errorf("authorize Twitch subscription access: %w", err)
	}
	o.credentials = &credentials
	o.userID = ""
	o.validatedAt = time.Time{}
	if err := o.validate(ctx); err != nil {
		o.credentials = previousCredentials
		o.userID = previousUserID
		o.validatedAt = previousValidatedAt
		return fmt.Errorf("validate upgraded Twitch authorization: %w", err)
	}
	if err := o.Store.Save(credentials); err != nil {
		o.credentials = previousCredentials
		o.userID = previousUserID
		o.validatedAt = previousValidatedAt
		return fmt.Errorf("save upgraded Twitch authorization: %w", err)
	}
	return nil
}

type CredentialsWithExpiry struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (o *OAuth) refresh(ctx context.Context) error {
	form := url.Values{"client_id": {o.ClientID}, "refresh_token": {o.credentials.RefreshToken}, "grant_type": {"refresh_token"}}
	var c CredentialsWithExpiry
	if err := o.postForm(ctx, o.baseURL()+"/token", form, &c); err != nil {
		var responseErr *oauthResponseError
		if errors.As(err, &responseErr) && (responseErr.Status == http.StatusBadRequest || responseErr.Status == http.StatusUnauthorized) {
			return fmt.Errorf("%w: %s", errRefreshRejected, responseErr.Message)
		}
		return err
	}
	if c.AccessToken == "" || c.RefreshToken == "" {
		return errors.New("Twitch returned an incomplete refresh response")
	}
	updated := Credentials{AccessToken: c.AccessToken, RefreshToken: c.RefreshToken}
	if err := o.Store.Save(updated); err != nil {
		return err
	}
	o.credentials = &updated
	return nil
}

func (o *OAuth) RefreshAfterUnauthorized(ctx context.Context) error {
	if o.credentials == nil {
		return errInvalidToken
	}
	return o.refreshOrLogin(ctx)
}

func (o *OAuth) refreshOrLogin(ctx context.Context) error {
	err := o.refresh(ctx)
	if err != nil && !errors.Is(err, errRefreshRejected) {
		return fmt.Errorf("refresh Twitch authorization: %w", err)
	}
	if errors.Is(err, errRefreshRejected) {
		if deleteErr := o.Store.Delete(); deleteErr != nil {
			return fmt.Errorf("saved Twitch authorization expired and could not be removed: %w", deleteErr)
		}
		o.credentials = nil
		credentials, loginErr := o.deviceLogin(ctx)
		if loginErr != nil {
			return fmt.Errorf("saved Twitch authorization expired; device sign-in failed: %w", loginErr)
		}
		o.credentials = &credentials
	}
	o.validatedAt = time.Time{}
	if err := o.validate(ctx); err != nil {
		return fmt.Errorf("validate renewed Twitch token: %w", err)
	}
	return nil
}

func (o *OAuth) postForm(ctx context.Context, endpoint string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(resp.StatusCode)
		}
		return &oauthResponseError{Status: resp.StatusCode, Message: apiErr.Message}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode OAuth response: %w", err)
	}
	return nil
}
func has(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
