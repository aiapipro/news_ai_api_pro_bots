package allainews

import (
	"fmt"
	"net/http"
	"net/url"
	"newsbots/pkg/posts"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
)

var aiNewsAllowedHosts = []string{"towardsdatascience.com", "aihub.org"}

func GetPostsFromRSS() (posts.Posts, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://allainews.com/feed/")
	if err != nil {
		return nil, fmt.Errorf("could not get allainews rss feed: %w", err)
	}
	allainewsPosts := make(posts.Posts, 0)
	for _, i := range feed.Items {
		res, err := http.Get(i.Link)
		if err != nil {
			return nil, fmt.Errorf("could not http get page: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			continue
		}
		doc, err := goquery.NewDocumentFromReader(res.Body)
		if err != nil {
			return nil, fmt.Errorf("could NewDocumentFromReader: %w", err)
		}

		urlString := ""
		doc.Find("a.btn-primary").Each(func(i int, s *goquery.Selection) {
			if strings.TrimSpace(s.Text()) != "Visit resource" {
				return
			}
			urlString, _ = s.Attr("href")

			u, err := url.Parse(urlString)
			if err == nil {
				newQuery := u.Query()
				for k, _ := range newQuery {
					if k == "source" || strings.HasPrefix(k, "utm_") {
						newQuery.Del(k)
					}
				}
				u.RawQuery = newQuery.Encode()
				urlString = u.String()
			}
		})

		if urlString != "" {
			for _, allowedDomain := range aiNewsAllowedHosts {
				if strings.Contains(urlString, allowedDomain) {
					allainewsPosts = append(allainewsPosts, posts.Post{
						Title: i.Title,
						Url:   urlString,
					})
					break
				}
			}
		}
	}

	return allainewsPosts, nil
}
