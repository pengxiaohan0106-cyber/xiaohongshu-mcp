package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/errors"
)

type FeedsListAction struct {
	page *rod.Page
}

func NewFeedsListAction(page *rod.Page) *FeedsListAction {
	return &FeedsListAction{page: page}
}

// safeNavigate navigates with explicit timeout and error handling
func safeNavigateFeed(page *rod.Page, url string, timeout time.Duration) error {
	page = page.Timeout(timeout)
	rod.Try(func() {
		page.MustNavigate(url)
	})
	return nil
}

// safeWaitLoad waits for page load with timeout
func safeWaitLoad(page *rod.Page, timeout time.Duration) {
	page = page.Timeout(timeout)
	rod.Try(func() {
		page.MustWaitLoad()
	})
}

// GetFeedsList 获取页面的 Feed 列表数据
func (f *FeedsListAction) GetFeedsList(ctx context.Context) ([]Feed, error) {
	page := f.page.Context(ctx)

	// Navigate with timeout (key fix - original had no timeout!)
	if err := safeNavigateFeed(page, "https://www.xiaohongshu.com", 20*time.Second); err != nil {
		return nil, fmt.Errorf("navigation failed: %w", err)
	}

	// Wait for page load with timeout (key fix - MustWaitDOMStable blocks forever!)
	safeWaitLoad(page, 15*time.Second)

	result := ""
	rod.Try(func() {
		result = page.Timeout(10 * time.Second).MustEval(`() => {
			if (window.__INITIAL_STATE__ &&
			    window.__INITIAL_STATE__.feed &&
			    window.__INITIAL_STATE__.feed.feeds) {
				const feeds = window.__INITIAL_STATE__.feed.feeds;
				const feedsData = feeds.value !== undefined ? feeds.value : feeds._value;
				if (feedsData) {
					return JSON.stringify(feedsData);
				}
			}
			return "";
		}`).String()
	})

	if result == "" {
		return nil, errors.ErrNoFeeds
	}

	var feeds []Feed
	if err := json.Unmarshal([]byte(result), &feeds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal feeds: %w", err)
	}

	return feeds, nil
}
