package idp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type wecomProvider struct {
	cfg  Config
	http *http.Client
}

func newWeCom(cfg Config) (*wecomProvider, error) {
	if cfg["corpId"] == "" || cfg["secret"] == "" {
		return nil, fmt.Errorf("corpId and secret are required")
	}
	return &wecomProvider{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (p *wecomProvider) AuthURL(state, redirectURI string) string {
	q := url.Values{
		"appid":         {p.cfg["corpId"]},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"snsapi_base"},
		"state":         {state},
		"agentid":       {p.cfg["agentId"]},
	}
	return "https://open.weixin.qq.com/connect/oauth2/authorize?" + q.Encode() + "#wechat_redirect"
}

func (p *wecomProvider) Exchange(ctx context.Context, code, _ string) (*Identity, error) {
	tokenURL := "https://qyapi.weixin.qq.com/cgi-bin/gettoken?" + url.Values{
		"corpid":     {p.cfg["corpId"]},
		"corpsecret": {p.cfg["secret"]},
	}.Encode()
	var token struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := getJSON(ctx, p.http, tokenURL, &token); err != nil {
		return nil, err
	}
	if token.ErrCode != 0 || token.AccessToken == "" {
		return nil, fmt.Errorf("wecom token error %d: %s", token.ErrCode, token.ErrMsg)
	}
	userURL := "https://qyapi.weixin.qq.com/cgi-bin/user/getuserinfo?" + url.Values{
		"access_token": {token.AccessToken},
		"code":         {code},
	}.Encode()
	var info struct {
		UserID  string `json:"UserId"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := getJSON(ctx, p.http, userURL, &info); err != nil {
		return nil, err
	}
	if info.ErrCode != 0 || info.UserID == "" {
		return nil, fmt.Errorf("wecom userinfo error %d: %s", info.ErrCode, info.ErrMsg)
	}
	detailURL := "https://qyapi.weixin.qq.com/cgi-bin/user/get?" + url.Values{
		"access_token": {token.AccessToken},
		"userid":       {info.UserID},
	}.Encode()
	var detail struct {
		UserID  string `json:"userid"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Mobile  string `json:"mobile"`
		Avatar  string `json:"avatar"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := getJSON(ctx, p.http, detailURL, &detail); err != nil {
		return nil, err
	}
	if detail.ErrCode != 0 {
		return nil, fmt.Errorf("wecom user detail error %d: %s", detail.ErrCode, detail.ErrMsg)
	}
	return &Identity{
		Subject: info.UserID,
		Email:   detail.Email,
		Phone:   detail.Mobile,
		Name:    detail.Name,
		Avatar:  detail.Avatar,
		Raw:     map[string]interface{}{"userid": detail.UserID, "name": detail.Name, "email": detail.Email, "mobile": detail.Mobile},
	}, nil
}
