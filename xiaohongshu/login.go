package xiaohongshu

import (
	"context"
	"time"

	"github.com/go-rod/rod"
	"github.com/pkg/errors"
)

type LoginAction struct {
	page *rod.Page
}

func NewLogin(page *rod.Page) *LoginAction {
	return &LoginAction{page: page}
}

// safeNavigate navigates with explicit timeout and error handling
func safeNavigateLogin(page *rod.Page, url string, timeout time.Duration) error {
	page = page.Timeout(timeout)
	rod.Try(func() {
		page.MustNavigate(url)
	})
	return nil
}

// safeWaitLoad waits for page load with timeout
func safeWaitLoadLogin(page *rod.Page, timeout time.Duration) {
	page = page.Timeout(timeout)
	rod.Try(func() {
		page.MustWaitLoad()
	})
}

// safeHas checks element with timeout
func safeHasLogin(page *rod.Page, selector string, timeout time.Duration) bool {
	page = page.Timeout(timeout)
	var exists bool
	rod.Try(func() {
		exists, _, _ = page.Has(selector)
	})
	return exists
}

// safeElement gets element with timeout
func safeElementLogin(page *rod.Page, selector string, timeout time.Duration) (*rod.Element, error) {
	page = page.Timeout(timeout)
	var el *rod.Element
	var err error
	rod.Try(func() {
		el, err = page.Element(selector)
		if err != nil {
			el = nil
		}
	})
	if el == nil {
		return nil, err
	}
	return el, nil
}

func (a *LoginAction) CheckLoginStatus(ctx context.Context) (bool, error) {
	pp := a.page.Context(ctx)

	// Navigate with timeout
	if err := safeNavigateLogin(pp, "https://www.xiaohongshu.com/explore", 20*time.Second); err != nil {
		return false, errors.Wrap(err, "navigation failed")
	}

	// Wait for page load with timeout
	safeWaitLoadLogin(pp, 15*time.Second)

	// Check with timeout
	if !safeHasLogin(pp, `.main-container .user .link-wrapper .channel`, 10*time.Second) {
		return false, errors.New("login status element not found")
	}

	return true, nil
}

func (a *LoginAction) Login(ctx context.Context) error {
	pp := a.page.Context(ctx)

	// Navigate with timeout
	if err := safeNavigateLogin(pp, "https://www.xiaohongshu.com/explore", 20*time.Second); err != nil {
		return errors.Wrap(err, "navigation failed")
	}

	// Wait for page load with timeout
	safeWaitLoadLogin(pp, 15*time.Second)

	// Check if already logged in
	if safeHasLogin(pp, ".main-container .user .link-wrapper .channel", 5*time.Second) {
		return nil
	}

	return nil
}

func (a *LoginAction) FetchQrcodeImage(ctx context.Context) (string, bool, error) {
	pp := a.page.Context(ctx)

	// Navigate with timeout
	if err := safeNavigateLogin(pp, "https://www.xiaohongshu.com/explore", 20*time.Second); err != nil {
		return "", false, errors.Wrap(err, "navigation failed")
	}

	// Wait for page load with timeout
	safeWaitLoadLogin(pp, 15*time.Second)

	// Check if already logged in
	if safeHasLogin(pp, ".main-container .user .link-wrapper .channel", 5*time.Second) {
		return "", true, nil
	}

	// Get QR code image with timeout
	el, err := safeElementLogin(pp, ".login-container .qrcode-img", 10*time.Second)
	if err != nil {
		return "", false, errors.Wrap(err, "get qrcode element failed")
	}

	src, err := el.Attribute("src")
	if err != nil || src == nil || len(*src) == 0 {
		return "", false, errors.New("qrcode src is empty")
	}

	return *src, false, nil
}

func (a *LoginAction) WaitForLogin(ctx context.Context) bool {
	pp := a.page.Context(ctx)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(120 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return false
		case <-timeout:
			return false
		case <-ticker.C:
			el, err := pp.Element(".main-container .user .link-wrapper .channel")
			if err == nil && el != nil {
				return true
			}
		}
	}
}
