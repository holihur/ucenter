package idp

import (
	"context"
	"strings"
)

type Config map[string]string

type Identity struct {
	Subject       string
	UnionID       string
	Email         string
	EmailVerified bool
	Phone         string
	Name          string
	Avatar        string
	Raw           map[string]interface{}
}

type Provider interface {
	AuthURL(state, redirectURI string) string
	Exchange(ctx context.Context, code, redirectURI string) (*Identity, error)
}

type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
}

type Definition struct {
	Type        string  `json:"type"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Mode        string  `json:"mode"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
}

var Catalog = []Definition{
	{Type: "wechat_open", Name: "WeChat Open Platform", Category: "China", Mode: "qr", Description: "PC QR login via WeChat Open Platform.", Fields: []Field{
		{Key: "appId", Label: "AppID", Required: true}, {Key: "appSecret", Label: "AppSecret", Secret: true, Required: true},
		{Key: "scope", Label: "Scope", Placeholder: "snsapi_login"},
	}},
	{Type: "wechat_mp", Name: "WeChat Official Account", Category: "China", Mode: "redirect", Description: "H5 login inside WeChat.", Fields: []Field{
		{Key: "appId", Label: "AppID", Required: true}, {Key: "appSecret", Label: "AppSecret", Secret: true, Required: true},
		{Key: "scope", Label: "Scope", Placeholder: "snsapi_userinfo"},
	}},
	{Type: "wecom", Name: "WeCom", Category: "China", Mode: "redirect", Description: "Enterprise WeChat OAuth login.", Fields: []Field{
		{Key: "corpId", Label: "CorpID", Required: true}, {Key: "agentId", Label: "AgentID", Required: true},
		{Key: "secret", Label: "Corp Secret", Secret: true, Required: true},
	}},
	{Type: "alipay", Name: "Alipay", Category: "China", Mode: "redirect", Description: "Alipay web authorization with RSA2.", Fields: []Field{
		{Key: "appId", Label: "AppID", Required: true}, {Key: "privateKey", Label: "App Private Key", Secret: true, Required: true},
		{Key: "alipayPublicKey", Label: "Alipay Public Key", Secret: true},
		{Key: "gateway", Label: "Gateway", Placeholder: "https://openapi.alipay.com/gateway.do"},
	}},
	{Type: "dingtalk", Name: "DingTalk", Category: "China", Mode: "redirect", Description: "DingTalk OAuth2 login.", Fields: generalFields()},
	{Type: "feishu", Name: "Feishu / Lark", Category: "China", Mode: "redirect", Description: "Feishu OAuth2 login.", Fields: generalFields()},
	{Type: "qq", Name: "QQ", Category: "China", Mode: "redirect", Description: "QQ Connect OAuth2 login.", Fields: generalFields()},
	{Type: "weibo", Name: "Weibo", Category: "China", Mode: "redirect", Description: "Weibo OAuth2 login.", Fields: generalFields()},
	{Type: "douyin", Name: "Douyin", Category: "China", Mode: "redirect", Description: "Douyin Open Platform OAuth2 login.", Fields: generalFields()},
	{Type: "google", Name: "Google", Category: "Global", Mode: "redirect", Description: "Google OIDC login.", Fields: generalFields()},
	{Type: "microsoft", Name: "Microsoft", Category: "Global", Mode: "redirect", Description: "Microsoft Entra ID login.", Fields: generalFields()},
	{Type: "github", Name: "GitHub", Category: "Global", Mode: "redirect", Description: "GitHub OAuth login.", Fields: generalFields()},
	{Type: "apple", Name: "Apple", Category: "Global", Mode: "redirect", Description: "Sign in with Apple.", Fields: generalFields()},
	{Type: "custom_oauth2", Name: "Custom OAuth 2.0", Category: "Custom", Mode: "redirect", Description: "Any OAuth2 provider with configurable endpoints.", Fields: generalFields()},
	{Type: "custom_oidc", Name: "Custom OIDC", Category: "Custom", Mode: "redirect", Description: "Any OpenID Connect provider.", Fields: generalFields()},
	{Type: "mock", Name: "Mock Provider", Category: "Development", Mode: "redirect", Description: "Local test provider that returns a fixed identity.", Fields: []Field{
		{Key: "subject", Label: "Subject", Placeholder: "mock-user"}, {Key: "email", Label: "Email", Placeholder: "mock@example.com"},
		{Key: "name", Label: "Display name", Placeholder: "Mock User"},
	}},
}

func generalFields() []Field {
	return []Field{
		{Key: "clientId", Label: "Client ID / App ID", Required: true},
		{Key: "clientSecret", Label: "Client Secret", Secret: true, Required: true},
		{Key: "authorizeUrl", Label: "Authorization URL", Required: true},
		{Key: "tokenUrl", Label: "Token URL", Required: true},
		{Key: "userinfoUrl", Label: "UserInfo URL", Required: true},
		{Key: "scopes", Label: "Scopes", Placeholder: "openid profile email"},
		{Key: "userIdField", Label: "Subject field", Placeholder: "sub"},
		{Key: "emailField", Label: "Email field", Placeholder: "email"},
		{Key: "nameField", Label: "Name field", Placeholder: "name"},
		{Key: "avatarField", Label: "Avatar field", Placeholder: "picture"},
	}
}

func DefinitionFor(typ string) (Definition, bool) {
	for _, d := range Catalog {
		if d.Type == typ {
			return d, true
		}
	}
	return Definition{}, false
}

func Types() []string {
	out := make([]string, 0, len(Catalog))
	for _, d := range Catalog {
		out = append(out, d.Type)
	}
	return out
}

func IsSupported(typ string) bool {
	_, ok := DefinitionFor(typ)
	return ok
}

func New(typ string, cfg Config) (Provider, error) {
	switch typ {
	case "wechat_open", "wechat_mp":
		return newWeChat(typ, cfg)
	case "wecom":
		return newWeCom(cfg)
	case "alipay":
		return newAlipay(cfg)
	case "mock":
		return &mockProvider{cfg: cfg}, nil
	default:
		if !IsSupported(typ) {
			return nil, ErrUnsupported
		}
		return newOAuth2(typ, cfg)
	}
}

var ErrUnsupported = &UnsupportedError{}

type UnsupportedError struct{}

func (*UnsupportedError) Error() string { return "unsupported identity provider" }

func trimSlash(s string) string { return strings.TrimRight(strings.TrimSpace(s), "/") }
