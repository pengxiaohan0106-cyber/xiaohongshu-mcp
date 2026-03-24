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

// FilterOption 筛选选项结构体
type FilterOption struct {
	SortBy      string `json:"sort_by,omitempty"`
	NoteType    string `json:"note_type,omitempty"`
	PublishTime string `json:"publish_time,omitempty"`
	SearchScope string `json:"search_scope,omitempty"`
	Location    string `json:"location,omitempty"`
}

// internalFilterOption 内部使用的筛选选项(基于索引)
type internalFilterOption struct {
	FiltersIndex int
	TagsIndex    int
	Text         string
}

var filterOptionsMap = map[int][]internalFilterOption{
	1: {
		{FiltersIndex: 1, TagsIndex: 1, Text: "综合"},
		{FiltersIndex: 1, TagsIndex: 2, Text: "最新"},
		{FiltersIndex: 1, TagsIndex: 3, Text: "最多点赞"},
		{FiltersIndex: 1, TagsIndex: 4, Text: "最多评论"},
		{FiltersIndex: 1, TagsIndex: 5, Text: "最多收藏"},
	},
	2: {
		{FiltersIndex: 2, TagsIndex: 1, Text: "不限"},
		{FiltersIndex: 2, TagsIndex: 2, Text: "视频"},
		{FiltersIndex: 2, TagsIndex: 3, Text: "图文"},
	},
	3: {
		{FiltersIndex: 3, TagsIndex: 1, Text: "不限"},
		{FiltersIndex: 3, TagsIndex: 2, Text: "一天内"},
		{FiltersIndex: 3, TagsIndex: 3, Text: "一周内"},
		{FiltersIndex: 3, TagsIndex: 4, Text: "半年内"},
	},
	4: {
		{FiltersIndex: 4, TagsIndex: 1, Text: "不限"},
		{FiltersIndex: 4, TagsIndex: 2, Text: "已看过"},
		{FiltersIndex: 4, TagsIndex: 3, Text: "未看过"},
		{FiltersIndex: 4, TagsIndex: 4, Text: "已关注"},
	},
	5: {
		{FiltersIndex: 5, TagsIndex: 1, Text: "不限"},
		{FiltersIndex: 5, TagsIndex: 2, Text: "同城"},
		{FiltersIndex: 5, TagsIndex: 3, Text: "附近"},
	},
}

func convertToInternalFilters(filter FilterOption) ([]internalFilterOption, error) {
	var internalFilters []internalFilterOption
	if filter.SortBy != "" {
		internal, err := findInternalOption(1, filter.SortBy)
		if err != nil {
			return nil, fmt.Errorf("排序依据错误: %w", err)
		}
		internalFilters = append(internalFilters, internal)
	}
	if filter.NoteType != "" {
		internal, err := findInternalOption(2, filter.NoteType)
		if err != nil {
			return nil, fmt.Errorf("笔记类型错误: %w", err)
		}
		internalFilters = append(internalFilters, internal)
	}
	if filter.PublishTime != "" {
		internal, err := findInternalOption(3, filter.PublishTime)
		if err != nil {
			return nil, fmt.Errorf("发布时间错误: %w", err)
		}
		internalFilters = append(internalFilters, internal)
	}
	if filter.SearchScope != "" {
		internal, err := findInternalOption(4, filter.SearchScope)
		if err != nil {
			return nil, fmt.Errorf("搜索范围错误: %w", err)
		}
		internalFilters = append(internalFilters, internal)
	}
	if filter.Location != "" {
		internal, err := findInternalOption(5, filter.Location)
		if err != nil {
			return nil, fmt.Errorf("位置距离错误: %w", err)
		}
		internalFilters = append(internalFilters, internal)
	}
	return internalFilters, nil
}

func findInternalOption(filtersIndex int, text string) (internalFilterOption, error) {
	options, exists := filterOptionsMap[filtersIndex]
	if !exists {
		return internalFilterOption{}, fmt.Errorf("筛选组 %d 不存在", filtersIndex)
	}
	for _, option := range options {
		if option.Text == text {
			return option, nil
		}
	}
	return internalFilterOption{}, fmt.Errorf("在筛选组 %d 中未找到文本 '%s'", filtersIndex, text)
}

func validateInternalFilterOption(filter internalFilterOption) error {
	if filter.FiltersIndex < 1 || filter.FiltersIndex > 5 {
		return fmt.Errorf("无效的筛选组索引 %d，有效范围为 1-5", filter.FiltersIndex)
	}
	options, exists := filterOptionsMap[filter.FiltersIndex]
	if !exists {
		return fmt.Errorf("筛选组 %d 不存在", filter.FiltersIndex)
	}
	if filter.TagsIndex < 1 || filter.TagsIndex > len(options) {
		return fmt.Errorf("筛选组 %d 的标签索引 %d 超出范围", filter.FiltersIndex, filter.TagsIndex)
	}
	return nil
}

type SearchAction struct {
	page *rod.Page
}

func NewSearchAction(page *rod.Page) *SearchAction {
	return &SearchAction{page: page}
}

func safeNavigate(page *rod.Page, urlStr string, timeout time.Duration) error {
	page = page.Timeout(timeout)
	rod.Try(func() {
		page.MustNavigate(urlStr)
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

	// Wait for page load with explicit timeout (not WaitStable which can block forever)
	logrus.Infof("[Search] Waiting for page load")
	page.Timeout(15 * time.Second).WaitLoad()

	// Wait for __INITIAL_STATE__ with 30s timeout (key fix - original had no timeout!)
	stateFound := safeWait(page, `() => window.__INITIAL_STATE__ !== undefined && window.__INITIAL_STATE__.search !== undefined`, 30*time.Second)
	if !stateFound {
		logrus.Warnf("[Search] __INITIAL_STATE__ not found within 30s, trying fallback")
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
	// Try to extract from page source directly
	rod.Try(func() {
		page.Timeout(10 * time.Second).WaitLoad()
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
