package idp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type oauth2Provider struct {
	typ  string
	cfg  Config
	http *http.Client
}

type preset struct {
	authorizeURL, tokenURL, userinfoURL, scopes                   string
	tokenMethod, tokenFormat, authStyle                           string
	tokenField, clientIDParam, clientSecretParam                  string
	userinfoAuth, userinfoParams                                  string
	userIDField, emailField, nameField, avatarField, unionIDField string
}

var presets = map[string]preset{
	"google": {
		authorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		tokenURL:     "https://oauth2.googleapis.com/token",
		userinfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
		scopes:       "openid profile email",
		tokenMethod:  "POST", tokenFormat: "form", authStyle: "post",
		userinfoAuth: "bearer", userIDField: "sub", emailField: "email", nameField: "name", avatarField: "picture",
	},
	"microsoft": {
		authorizeURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		tokenURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		userinfoURL:  "https://graph.microsoft.com/oidc/userinfo",
		scopes:       "openid profile email",
		tokenMethod:  "POST", tokenFormat: "form", authStyle: "post",
		userinfoAuth: "bearer", userIDField: "sub", emailField: "email", nameField: "name", avatarField: "picture",
	},
	"github": {
		authorizeURL: "https://github.com/login/oauth/authorize",
		tokenURL:     "https://github.com/login/oauth/access_token",
		userinfoURL:  "https://api.github.com/user",
		scopes:       "read:user user:email",
		tokenMethod:  "POST", tokenFormat: "form", authStyle: "post",
		userinfoAuth: "bearer", userIDField: "id", emailField: "email", nameField: "name", avatarField: "avatar_url",
	},
	"dingtalk": {
		authorizeURL: "https://login.dingtalk.com/oauth2/auth",
		tokenURL:     "https://api.dingtalk.com/v1.0/oauth2/userAccessToken",
		userinfoURL:  "https://api.dingtalk.com/v1.0/contact/users/me",
		scopes:       "openid",
		tokenMethod:  "POST", tokenFormat: "json", authStyle: "body",
		tokenField: "accessToken", clientIDParam: "clientId", clientSecretParam: "clientSecret",
		userinfoAuth: "bearer", userIDField: "unionId", emailField: "email", nameField: "nick", avatarField: "avatarUrl",
	},
	"feishu": {
		authorizeURL: "https://open.feishu.cn/open-apis/authen/v1/authorize",
		tokenURL:     "https://open.feishu.cn/open-apis/authen/v1/oidc/access_token",
		userinfoURL:  "https://open.feishu.cn/open-apis/authen/v1/user_info",
		scopes:       "openid",
		tokenMethod:  "POST", tokenFormat: "json", authStyle: "body",
		tokenField: "access_token", clientIDParam: "client_id", clientSecretParam: "client_secret",
		userinfoAuth: "bearer", userIDField: "union_id", emailField: "email", nameField: "name", avatarField: "avatar_url",
	},
	"weibo": {
		authorizeURL: "https://api.weibo.com/oauth2/authorize",
		tokenURL:     "https://api.weibo.com/oauth2/access_token",
		userinfoURL:  "https://api.weibo.com/2/users/show.json?uid={uid}",
		scopes:       "email",
		tokenMethod:  "POST", tokenFormat: "form", authStyle: "post",
		userinfoAuth: "query", userIDField: "idstr", emailField: "email", nameField: "screen_name", avatarField: "avatar_large",
	},
	"douyin": {
		authorizeURL: "https://open.douyin.com/platform/oauth/connect/",
		tokenURL:     "https://open.douyin.com/oauth/access_token/",
		userinfoURL:  "https://open.douyin.com/oauth/userinfo/",
		scopes:       "user_info",
		tokenMethod:  "POST", tokenFormat: "json", authStyle: "body",
		tokenField:   "data.access_token",
		userinfoAuth: "bearer", userIDField: "data.open_id", emailField: "data.email", nameField: "data.nickname", avatarField: "data.avatar",
	},
	"qq": {
		authorizeURL: "https://graph.qq.com/oauth2.0/authorize",
		tokenURL:     "https://graph.qq.com/oauth2.0/token",
		userinfoURL:  "https://graph.qq.com/user/get_user_info",
		scopes:       "get_user_info",
		tokenMethod:  "GET", tokenFormat: "query", authStyle: "query",
		userinfoAuth: "query", userIDField: "openid", emailField: "email", nameField: "nickname", avatarField: "figureurl_qq_2",
	},
	"apple": {
		authorizeURL: "https://appleid.apple.com/auth/authorize",
		tokenURL:     "https://appleid.apple.com/auth/token",
		userinfoURL:  "",
		scopes:       "name email",
		tokenMethod:  "POST", tokenFormat: "form", authStyle: "post",
		userinfoAuth: "bearer", userIDField: "sub", emailField: "email", nameField: "name",
	},
}

func newOAuth2(typ string, cfg Config) (*oauth2Provider, error) {
	p := presets[typ]
	get := func(key, fallback string) string {
		if v := strings.TrimSpace(cfg[key]); v != "" {
			return v
		}
		return fallback
	}
	cfg["authorizeUrl"] = get("authorizeUrl", p.authorizeURL)
	cfg["tokenUrl"] = get("tokenUrl", p.tokenURL)
	cfg["userinfoUrl"] = get("userinfoUrl", p.userinfoURL)
	cfg["scopes"] = get("scopes", p.scopes)
	cfg["tokenMethod"] = get("tokenMethod", orDefault(p.tokenMethod, "POST"))
	cfg["tokenFormat"] = get("tokenFormat", orDefault(p.tokenFormat, "form"))
	cfg["authStyle"] = get("authStyle", orDefault(p.authStyle, "post"))
	cfg["tokenField"] = get("tokenField", orDefault(p.tokenField, "access_token"))
	cfg["clientIdParam"] = get("clientIdParam", orDefault(p.clientIDParam, "client_id"))
	cfg["clientSecretParam"] = get("clientSecretParam", orDefault(p.clientSecretParam, "client_secret"))
	cfg["userinfoAuth"] = get("userinfoAuth", orDefault(p.userinfoAuth, "bearer"))
	cfg["userIdField"] = get("userIdField", orDefault(p.userIDField, "sub"))
	cfg["emailField"] = get("emailField", orDefault(p.emailField, "email"))
	cfg["nameField"] = get("nameField", orDefault(p.nameField, "name"))
	cfg["avatarField"] = get("avatarField", orDefault(p.avatarField, "picture"))
	cfg["unionIdField"] = get("unionIdField", p.unionIDField)
	if cfg["authorizeUrl"] == "" || cfg["tokenUrl"] == "" {
		return nil, errors.New("authorizeUrl and tokenUrl are required")
	}
	return &oauth2Provider{typ: typ, cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}, nil
}

func orDefault(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func (p *oauth2Provider) AuthURL(state, redirectURI string) string {
	u, err := url.Parse(p.cfg["authorizeUrl"])
	if err != nil {
		return p.cfg["authorizeUrl"]
	}
	q := u.Query()
	q.Set("client_id", p.cfg["clientId"])
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	if p.cfg["scopes"] != "" {
		q.Set("scope", p.cfg["scopes"])
	}
	if extra := p.cfg["extraAuthParams"]; extra != "" {
		if ev, err := url.ParseQuery(extra); err == nil {
			for k, vals := range ev {
				for _, v := range vals {
					q.Set(k, v)
				}
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *oauth2Provider) Exchange(ctx context.Context, code, redirectURI string) (*Identity, error) {
	tokenData, err := p.exchangeToken(ctx, code, redirectURI)
	if err != nil {
		return nil, err
	}
	accessToken := stringAt(tokenData, p.cfg["tokenField"])
	if accessToken == "" {
		accessToken = stringAt(tokenData, "access_token")
	}
	if accessToken == "" {
		accessToken = stringAt(tokenData, "accessToken")
	}
	if accessToken == "" {
		return nil, fmt.Errorf("token response did not contain an access token")
	}
	profile, err := p.fetchProfile(ctx, accessToken, tokenData)
	if err != nil {
		return nil, err
	}
	identity := &Identity{
		Subject:       stringAt(profile, p.cfg["userIdField"]),
		UnionID:       stringAt(profile, p.cfg["unionIdField"]),
		Email:         stringAt(profile, p.cfg["emailField"]),
		EmailVerified: boolAt(profile, "email_verified"),
		Phone:         stringAt(profile, "phone_number"),
		Name:          stringAt(profile, p.cfg["nameField"]),
		Avatar:        stringAt(profile, p.cfg["avatarField"]),
		Raw:           profile,
	}
	if identity.Subject == "" {
		return nil, fmt.Errorf("userinfo did not contain subject field %q", p.cfg["userIdField"])
	}
	return identity, nil
}

func (p *oauth2Provider) exchangeToken(ctx context.Context, code, redirectURI string) (map[string]interface{}, error) {
	values := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	if p.cfg["authStyle"] != "body" {
		values.Set(p.cfg["clientIdParam"], p.cfg["clientId"])
		values.Set(p.cfg["clientSecretParam"], p.cfg["clientSecret"])
	} else {
		values.Set(p.cfg["clientIdParam"], p.cfg["clientId"])
		values.Set(p.cfg["clientSecretParam"], p.cfg["clientSecret"])
	}
	method := strings.ToUpper(orDefault(p.cfg["tokenMethod"], "POST"))
	var req *http.Request
	var err error
	if method == "GET" {
		u, parseErr := url.Parse(p.cfg["tokenUrl"])
		if parseErr != nil {
			return nil, parseErr
		}
		q := u.Query()
		for k, vals := range values {
			for _, v := range vals {
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	} else if p.cfg["tokenFormat"] == "json" {
		body, _ := json.Marshal(valuesToMap(values))
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, p.cfg["tokenUrl"], strings.NewReader(string(body)))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, p.cfg["tokenUrl"], strings.NewReader(values.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Accept", "application/json")
		}
	}
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") || strings.HasPrefix(strings.TrimSpace(string(body)), "{") {
		var out map[string]interface{}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("decode token response: %w", err)
		}
		return out, nil
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	out := map[string]interface{}{}
	for k, v := range vals {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out, nil
}

func (p *oauth2Provider) fetchProfile(ctx context.Context, accessToken string, tokenData map[string]interface{}) (map[string]interface{}, error) {
	if p.cfg["userinfoUrl"] == "" {
		return tokenData, nil
	}
	rawURL := p.cfg["userinfoUrl"]
	for k, v := range tokenData {
		rawURL = strings.ReplaceAll(rawURL, "{"+k+"}", fmt.Sprint(v))
	}
	rawURL = strings.ReplaceAll(rawURL, "{access_token}", accessToken)
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if p.cfg["userinfoAuth"] == "query" {
		q.Set("access_token", accessToken)
	}
	if extra := p.cfg["userinfoParams"]; extra != "" {
		if ev, err := url.ParseQuery(extra); err == nil {
			for k, vals := range ev {
				for _, v := range vals {
					q.Set(k, v)
				}
			}
		}
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if p.cfg["userinfoAuth"] == "bearer" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}
	return out, nil
}

func valuesToMap(v url.Values) map[string]string {
	out := map[string]string{}
	for k, vals := range v {
		if len(vals) > 0 {
			out[k] = vals[0]
		}
	}
	return out
}

func stringAt(m map[string]interface{}, path string) string {
	if path == "" {
		return ""
	}
	var cur interface{} = m
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur = obj[part]
	}
	switch v := cur.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case bool:
		if v {
			return "true"
		}
	}
	return ""
}

func boolAt(m map[string]interface{}, path string) bool {
	cur := interface{}(m)
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return false
		}
		cur = obj[part]
	}
	switch v := cur.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	}
	return false
}
