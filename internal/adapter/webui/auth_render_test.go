package webui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui"
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
	if !strings.Contains(out, "glass") {
		t.Errorf("auth page not on glass: %s", out)
	}
}

func TestAuthPage_ForbiddenHasNoLoginCTA(t *testing.T) {
	out := render(t, webui.AuthPage(webui.AuthVM{TitleKey: "auth.forbidden.title", MsgKey: "auth.forbidden.msg", ShowLogin: false}))
	if strings.Contains(out, `href="/auth/login"`) {
		t.Errorf("forbidden page must not offer re-login: %s", out)
	}
}
