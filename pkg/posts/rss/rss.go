package rss

import (
	"fmt"
	"newsbots/pkg/posts"
	"strings"

	"github.com/mmcdole/gofeed"
)

func GetPostsFromRSS(rssFeed string) (posts.Posts, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(rssFeed)
	if err != nil {
		return nil, fmt.Errorf("could not get allainews rss feed: %w", err)
	}
	rssPosts := make(posts.Posts, 0)
	for _, i := range feed.Items {
		rssPosts = append(rssPosts, posts.Post{
			Title: strings.TrimSpace(i.Title),
			Url:   strings.TrimSpace(i.Link),
		})
	}

	return rssPosts, nil
}
