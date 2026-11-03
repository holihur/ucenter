package idp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type wechatProvider struct {
	typ  string
	cfg  Config
	http *http.Client
}

func newWeChat(typ string, cfg Config) (*wechatProvider, error) {
	if strings.TrimSpace(cfg["appId"]) == "" || strings.TrimSpace(cfg["appSecret"]) == "" {
		return nil, fmt.Errorf("appId and appSecret are required")
	}
	if cfg["scope"] == "" {
		if typ == "wechat_mp" {
			cfg["scope"] = "snsapi_userinfo"
		} else {
			cfg["scope"] = "snsapi_login"
		}
	}
	return &wechatProvider{typ: typ, cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (p *wechatProvider) AuthURL(state, redirectURI string) string {
	base := "https://open.weixin.qq.com/connect/qrconnect"
	if p.typ == "wechat_mp" {
		base = "https://open.weixin.qq.com/connect/oauth2/authorize"
	}
	q := url.Values{
		"appid":         {p.cfg["appId"]},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {p.cfg["scope"]},
		"state":         {state},
	}
	return base + "?" + q.Encode() + "#wechat_redirect"
}

func (p *wechatProvider) Exchange(ctx context.Context, code, _ string) (*Identity, error) {
	tokenURL := "https://api.weixin.qq.com/sns/oauth2/access_token?" + url.Values{
		"appid":      {p.cfg["appId"]},
		"secret":     {p.cfg["appSecret"]},
		"code":       {code},
		"grant_type": {"authorization_code"},
	}.Encode()
	var token struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
		UnionID     string `json:"unionid"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := getJSON(ctx, p.http, tokenURL, &token); err != nil {
		return nil, err
	}
	if token.ErrCode != 0 || token.AccessToken == "" {
		return nil, fmt.Errorf("wechat token error %d: %s", token.ErrCode, token.ErrMsg)
	}
	infoURL := "https://api.weixin.qq.com/sns/userinfo?" + url.Values{
		"access_token": {token.AccessToken},
		"openid":       {token.OpenID},
		"lang":         {"zh_CN"},
	}.Encode()
	var info struct {
		OpenID     string `json:"openid"`
		UnionID    string `json:"unionid"`
		Nickname   string `json:"nickname"`
		HeadImgURL string `json:"headimgurl"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := getJSON(ctx, p.http, infoURL, &info); err != nil {
		return nil, err
	}
	if info.ErrCode != 0 {
		return nil, fmt.Errorf("wechat userinfo error %d: %s", info.ErrCode, info.ErrMsg)
	}
	return &Identity{
		Subject: info.OpenID,
		UnionID: info.UnionID,
		Name:    info.Nickname,
		Avatar:  info.HeadImgURL,
		Raw:     map[string]interface{}{"openid": info.OpenID, "unionid": info.UnionID, "nickname": info.Nickname},
	}, nil
}

func getJSON(ctx context.Context, client *http.Client, rawURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("upstream returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode upstream response: %w", err)
	}
	return nil
}
