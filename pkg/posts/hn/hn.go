package hn

import (
	"fmt"
	"newsbots/pkg/posts"
	"strconv"

	"github.com/dgraph-io/badger/v4"
)

var hnAPIURL string = "https://hacker-news.firebaseio.com/v0"

func GetNew(db *badger.DB, maxElements int) (posts.Posts, error) {

	txn := db.NewTransaction(true)
	defer txn.Discard()

	key := "lastHNID"

	minId := -1
	v, err := txn.Get([]byte(key))
	if err == nil {
		minId, _ = strconv.Atoi(v.String())
	}

	storyIDs := make([]int, 0)
	err = posts.GetJSON(hnAPIURL+"/newstories.json", &storyIDs)
	if err != nil {
		return nil, fmt.Errorf("could not load newstories: %w", err)
	}
	if len(storyIDs) == 0 {
		return nil, fmt.Errorf("no single story found")
	}

	err = txn.Set([]byte(key), []byte(fmt.Sprintf("%d", storyIDs[0])))
	if err != nil {
		return nil, fmt.Errorf("could not set to db: %w", err)
	}

	// Commit the transaction and check for error.
	if err := txn.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit to db: %w", err)
	}

	returnPosts := make(posts.Posts, 0, len(storyIDs))
	for k, id := range storyIDs {
		if id < minId {
			break
		}
		if k > maxElements {
			break
		}

		p := posts.Post{}
		err := posts.GetJSON(fmt.Sprintf("%s/item/%d.json", hnAPIURL, id), &p)
		if err != nil {
			return nil, fmt.Errorf("could not load single story: %w", err)
		}
		if p.Url == "" {
			continue
		}

		returnPosts = append(returnPosts, p)

	}

	return returnPosts, nil
}
