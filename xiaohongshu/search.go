package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/errors"
	"github.com/sirupsen/logrus"
)

type SearchAction struct {
	page *rod.Page
}

func NewSearchAction(page *rod.Page) *SearchAction {
	return &SearchAction{page: page}
}

func safeNavigate(page *rod.Page, url string, timeout time.Duration) error {
	page = page.Timeout(timeout)
	rod.Try(func() {
		page.MustNavigate(url)
	})
	return nil
}

func safeWaitStable(page *rod.Page, timeout time.Duration) {
	page = page.Timeout(timeout)
	rod.Try(func() {
		page.MustWaitStable()
	})
}

func safeWait(page *rod.Page, js string, timeout time.Duration) bool {
	page = page.Timeout(timeout)
	var found bool
	rod.Try(func() {
		page.MustWait(js)
		found = true
	})
	return found
}

func (s *SearchAction) Search(ctx context.Context, keyword string, filters ...FilterOption) ([]Feed, error) {
	page := s.page.Context(ctx)
	searchURL := makeSearchURL(keyword)

	if err := safeNavigate(page, searchURL, 20*time.Second); err != nil {
		logrus.Warnf("[Search] Navigation failed: %v, retrying...", err)
		if retryErr := safeNavigate(page, searchURL, 30*time.Second); retryErr != nil {
			return nil, fmt.Errorf("navigation failed after retry: %w", retryErr)
		}
	}

	logrus.Infof("[Search] Waiting for DOM content")
	page.Timeout(15 * time.Second).WaitDOMContentLoaded()

	stateFound := safeWait(page, `() => window.__INITIAL_STATE__ !== undefined && window.__INITIAL_STATE__.search !== undefined`, 30*time.Second)
	if !stateFound {
		logrus.Warnf("[Search] __INITIAL_STATE__ not found within 30s")
		return s.tryFallbackSearch(page, keyword)
	}

	if len(filters) > 0 {
		var allFilters []internalFilterOption
		for _, f := range filters {
			internal, err := convertToInternalFilters(f)
			if err != nil {
				return nil, fmt.Errorf("filter convert failed: %w", err)
			}
			allFilters = append(allFilters, internal...)
		}
		for _, f := range allFilters {
			if err := validateInternalFilterOption(f); err != nil {
				return nil, fmt.Errorf("filter validate failed: %w", err)
			}
		}

		rod.Try(func() {
			btn := page.Timeout(5 * time.Second).MustElement(`div.filter`)
			btn.MustHover()
		})
		safeWait(page, `() => document.querySelector('div.filter-panel') !== null`, 10*time.Second)

		for _, f := range allFilters {
			selector := fmt.Sprintf(`div.filter-panel div.filters:nth-child(%d) div.tags:nth-child(%d)`,
				f.FiltersIndex, f.TagsIndex)
			rod.Try(func() {
				page.Timeout(5 * time.Second).MustElement(selector).MustClick()
			})
		}

		safeWaitStable(page, 10*time.Second)
		safeWait(page, `() => window.__INITIAL_STATE__ !== undefined`, 15*time.Second)
	}

	result := ""
	rod.Try(func() {
		result = page.Timeout(10 * time.Second).MustEval(`() => {
			if (window.__INITIAL_STATE__ &&
			    window.__INITIAL_STATE__.search &&
			    window.__INITIAL_STATE__.search.feeds) {
				const feeds = window.__INITIAL_STATE__.search.feeds;
				const data = feeds.value !== undefined ? feeds.value : feeds._value;
				return data ? JSON.stringify(data) : "";
			}
			return "";
		}`).String()
	})

	if result == "" {
		logrus.Warnf("[Search] Empty result, trying fallback")
		return s.tryFallbackSearch(page, keyword)
	}

	var feeds []Feed
	if err := json.Unmarshal([]byte(result), &feeds); err != nil {
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}

	logrus.Infof("[Search] Success: %d feeds", len(feeds))
	return feeds, nil
}

func (s *SearchAction) tryFallbackSearch(page *rod.Page, keyword string) ([]Feed, error) {
	rod.Try(func() {
		page.Timeout(10 * time.Second).WaitDOMContentLoaded()
	})

	result := ""
	rod.Try(func() {
		result = page.Timeout(10 * time.Second).MustEval(`() => {
			const items = document.querySelectorAll('.note-item, .feeds-page .item, [data-type="note"]');
			if (items.length > 0) {
				const feeds = [];
				items.forEach(item => {
					const titleEl = item.querySelector('.title, .content, .desc');
					const likeEl = item.querySelector('.likedCount, .like span');
					if (titleEl) {
						feeds.push({
							id: item.getAttribute('data-id') || item.id,
							title: titleEl.textContent.trim(),
							likedCount: likeEl ? parseInt(likeEl.textContent.replace(/[^0-9]/g,'')) : 0
						});
					}
				});
				if (feeds.length > 0) return JSON.stringify(feeds);
			}
			return "";
		}`).String()
	})

	if result != "" {
		var feeds []Feed
		if err := json.Unmarshal([]byte(result), &feeds); err != nil {
			logrus.Warnf("[Search] Fallback unmarshal failed: %v", err)
		} else if len(feeds) > 0 {
			logrus.Infof("[Search] Fallback success: %d feeds", len(feeds))
			return feeds, nil
		}
	}

	return nil, errors.ErrNoFeeds
}

func makeSearchURL(keyword string) string {
	values := url.Values{}
	values.Set("keyword", keyword)
	values.Set("source", "web_explore_feed")
	return fmt.Sprintf("https://www.xiaohongshu.com/search_result?%s", values.Encode())
}
