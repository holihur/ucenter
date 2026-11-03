package idp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type alipayProvider struct {
	cfg     Config
	http    *http.Client
	private *rsa.PrivateKey
	gateway string
}

func newAlipay(cfg Config) (*alipayProvider, error) {
	if cfg["appId"] == "" || cfg["privateKey"] == "" {
		return nil, fmt.Errorf("appId and privateKey are required")
	}
	key, err := parsePrivateKey(cfg["privateKey"])
	if err != nil {
		return nil, err
	}
	gateway := cfg["gateway"]
	if gateway == "" {
		gateway = "https://openapi.alipay.com/gateway.do"
	}
	return &alipayProvider{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}, private: key, gateway: gateway}, nil
}

func parsePrivateKey(value string) (*rsa.PrivateKey, error) {
	value = strings.ReplaceAll(value, "\\n", "\n")
	if block, _ := pem.Decode([]byte(value)); block != nil {
		value = string(block.Bytes)
	}
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, " ", "")
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode alipay private key: %w", err)
	}
	if key, err := x509.ParsePKCS8PrivateKey(raw); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	key, err := x509.ParsePKCS1PrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse alipay private key: %w", err)
	}
	return key, nil
}

func (p *alipayProvider) AuthURL(state, redirectURI string) string {
	auth := p.cfg["authorizeUrl"]
	if auth == "" {
		auth = "https://openauth.alipay.com/oauth2/publicAppAuthorize.htm"
	}
	q := url.Values{
		"app_id":       {p.cfg["appId"]},
		"scope":        {"auth_user"},
		"redirect_uri": {redirectURI},
		"state":        {state},
	}
	return auth + "?" + q.Encode()
}

func (p *alipayProvider) Exchange(ctx context.Context, code, _ string) (*Identity, error) {
	tokenResp, err := p.call(ctx, "alipay.system.oauth.token", map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	})
	if err != nil {
		return nil, err
	}
	accessToken := stringAt(tokenResp, "access_token")
	userID := stringAt(tokenResp, "user_id")
	if accessToken == "" || userID == "" {
		return nil, fmt.Errorf("alipay token response is missing access_token or user_id")
	}
	profile, err := p.call(ctx, "alipay.user.info.share", map[string]string{
		"auth_token": accessToken,
	})
	if err != nil {
		return nil, err
	}
	subject := stringAt(profile, "user_id")
	if subject == "" {
		subject = userID
	}
	return &Identity{
		Subject: subject,
		Name:    stringAt(profile, "nick_name"),
		Avatar:  stringAt(profile, "avatar"),
		Raw:     profile,
	}, nil
}

func (p *alipayProvider) call(ctx context.Context, method string, biz map[string]string) (map[string]interface{}, error) {
	params := map[string]string{
		"app_id":    p.cfg["appId"],
		"method":    method,
		"format":    "JSON",
		"charset":   "utf-8",
		"sign_type": "RSA2",
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
		"version":   "1.0",
	}
	for k, v := range biz {
		params[k] = v
	}
	sign, err := p.sign(params)
	if err != nil {
		return nil, err
	}
	params["sign"] = sign
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.gateway, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("alipay gateway returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode alipay response: %w", err)
	}
	key := strings.ReplaceAll(method, ".", "_") + "_response"
	node, _ := raw[key].(map[string]interface{})
	if node == nil {
		return nil, fmt.Errorf("alipay response missing %s: %s", key, strings.TrimSpace(string(body)))
	}
	if code := stringAt(node, "code"); code != "" && code != "10000" {
		return nil, fmt.Errorf("alipay error %s: %s", code, stringAt(node, "sub_msg"))
	}
	return node, nil
}

func (p *alipayProvider) sign(params map[string]string) (string, error) {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "&")))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.private, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}
