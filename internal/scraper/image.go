package scraper

import (
	"fmt"
	"net/url"
	"strconv"
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
	if mediaURL := extractFeedMedia(item); mediaURL != "" {
		return mediaURL
	}

	switch imageMode {
	case "none":
		return ""
	case "placeholder":
		return placeholderURL
	case "random":
		return fmt.Sprintf("https://api.dicebear.com/9.x/identicon/svg?seed=%s", url.QueryEscape(guid))
	case "extract":
		if imgURL := bestImgSrc(descHTML); imgURL != "" {
			return imgURL
		}
		return bestImgSrc(contentHTML)
	default:
		return ""
	}
}

// ExtractContentImage returns the best image URL from scraped HTML content.
// Used by the re-scrape path where no gofeed.Item is available.
func ExtractContentImage(contentHTML string) string {
	return bestImgSrc(contentHTML)
}

func extractFeedMedia(item *gofeed.Item) string {
	if item.Image != nil && item.Image.URL != "" {
		return item.Image.URL
	}

	if ext, ok := item.Extensions["media"]; ok {
		if thumbnails, ok := ext["thumbnail"]; ok && len(thumbnails) > 0 {
			if url := thumbnails[0].Attrs["url"]; url != "" {
				return url
			}
		}
		// Only use media:content if it is explicitly typed as an image;
		// many feeds include media:content for video/audio which is not a thumbnail.
		if contents, ok := ext["content"]; ok {
			for _, c := range contents {
				medium := c.Attrs["medium"]
				mimeType := c.Attrs["type"]
				isImage := medium == "image" || strings.HasPrefix(mimeType, "image/")
				if isImage {
					if url := c.Attrs["url"]; url != "" {
						return url
					}
				}
			}
		}
	}

	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" {
			return enc.URL
		}
	}

	return ""
}

// minThumbnailDim is the minimum width or height (in pixels) for an image to be
// considered a content image rather than a logo, icon, or tracking pixel.
const minThumbnailDim = 100

// bestImgSrc picks the best thumbnail image from HTML content. It prefers the
// image with the largest area (width * height from attributes), falling back to
// the first image if none have dimensions. Images with width or height below
// minThumbnailDim are skipped to avoid logos, avatars, and tracking pixels.
func bestImgSrc(html string) string {
	if html == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	var bestURL string
	var bestArea int
	var firstURL string

	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || src == "" {
			return
		}

		w := attrInt(s, "width")
		h := attrInt(s, "height")

		// Skip small images (logos, avatars, tracking pixels).
		if (w > 0 && w < minThumbnailDim) || (h > 0 && h < minThumbnailDim) {
			return
		}

		// Track first valid image as fallback when no dimensions are available.
		if firstURL == "" {
			firstURL = src
		}

		area := w * h
		if area > bestArea {
			bestArea = area
			bestURL = src
		}
	})

	if bestURL != "" {
		return bestURL
	}
	return firstURL
}

func attrInt(s *goquery.Selection, name string) int {
	v, exists := s.Attr(name)
	if !exists {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
