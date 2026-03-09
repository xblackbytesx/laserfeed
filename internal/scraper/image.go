package scraper

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
)

// ExtractThumbnail resolves a thumbnail URL for an article.
//
// Priority:
//  1. Feed-provided media (media:thumbnail, media:content, gofeed Image, image/* enclosure)
//     — returned unconditionally regardless of imageMode.
//  2. Fallback strategy from imageMode, used only when the feed provides nothing:
//     "none"        → return ""
//     "placeholder" → configured placeholder URL
//     "random"      → DiceBear identicon seeded by article GUID
//     "extract"     → first <img> found in description then content HTML
func ExtractThumbnail(item *gofeed.Item, descHTML, contentHTML, imageMode, placeholderURL, guid string) string {
	// Step 1: always prefer officially-provided feed media.
	if mediaURL := extractFeedMedia(item); mediaURL != "" {
		return mediaURL
	}

	// Step 2: nothing in the feed — apply the configured fallback strategy.
	switch imageMode {
	case "none":
		return ""
	case "placeholder":
		return placeholderURL
	case "random":
		return fmt.Sprintf("https://api.dicebear.com/7.x/identicon/svg?seed=%s", url.QueryEscape(guid))
	case "extract":
		if imgURL := firstImgSrc(descHTML); imgURL != "" {
			return imgURL
		}
		return firstImgSrc(contentHTML)
	default:
		return ""
	}
}

// extractFeedMedia checks gofeed-standard fields and common media extensions.
func extractFeedMedia(item *gofeed.Item) string {
	// gofeed unified Image field (covers <image> and some media tags)
	if item.Image != nil && item.Image.URL != "" {
		return item.Image.URL
	}

	// media:thumbnail and media:content namespace extensions
	if ext, ok := item.Extensions["media"]; ok {
		if thumbnails, ok := ext["thumbnail"]; ok && len(thumbnails) > 0 {
			if url := thumbnails[0].Attrs["url"]; url != "" {
				return url
			}
		}
		if contents, ok := ext["content"]; ok && len(contents) > 0 {
			if url := contents[0].Attrs["url"]; url != "" {
				return url
			}
		}
	}

	// RSS enclosure with an image MIME type
	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" {
			return enc.URL
		}
	}

	return ""
}

func firstImgSrc(html string) string {
	if html == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	src := ""
	doc.Find("img").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if v, exists := s.Attr("src"); exists && v != "" {
			src = v
			return false
		}
		return true
	})
	return src
}
