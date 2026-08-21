package webui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/i18n"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestAuthPage_LandingShowsLoginCTA(t *testing.T) {
	out := render(t, webui.AuthPage(webui.AuthVM{TitleKey: "auth.loggedOut.title", MsgKey: "auth.loggedOut.msg", ShowLogin: true}))
	if !strings.Contains(out, `href="/auth/login"`) {
		t.Errorf("landing missing login CTA: %s", out)
	}
	if !strings.Contains(out, "data-auth-page") {
		t.Errorf("auth page not on the Kasten card: %s", out)
	}
}

func TestAuthPage_ForbiddenHasNoLoginCTA(t *testing.T) {
	out := render(t, webui.AuthPage(webui.AuthVM{TitleKey: "auth.forbidden.title", MsgKey: "auth.forbidden.msg", ShowLogin: false}))
	if strings.Contains(out, `href="/auth/login"`) {
		t.Errorf("forbidden page must not offer re-login: %s", out)
	}
}

func TestAuthPage_NoSSEConnect(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	var b bytes.Buffer
	if err := webui.AuthPage(webui.AuthVM{TitleKey: "auth.loggedOut.title", MsgKey: "auth.loggedOut.msg", ShowLogin: true}).Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(b.String(), "sse-connect") {
		t.Fatalf("auth page must NOT open an SSE connection")
	}
}
