package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"newsbots/pkg/aiapipro"
	"newsbots/pkg/posts"
	"newsbots/pkg/posts/rss"
	"os"
	"path"
	"time"

	"github.com/dgraph-io/badger/v4"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type RSSFeedConfig struct {
	URL              string `json:"url"`
	CheckTitle       bool   `json:"check_title"`
	CheckLinkContent bool   `json:"check_link_content"`
	Username         string `json:"username"`
	MaxItems         *int   `json:"max_items,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("expect at least one command line argument")
	}

	binaryPath, err := os.Executable()
	if err != nil {
		log.Fatal("could not os.Executable", err)
	}
	binaryPath = path.Dir(binaryPath)

	opts := badger.DefaultOptions(path.Join(binaryPath, "badger.db"))
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal("could not badger open db:", err)
	}
	defer db.Close()

	// Load all current posts
	allCurrentPosts, err := aiapipro.GetPosts()
	if err != nil {
		fmt.Println("could not GetPostIDs", err)
		return
	}

	// Write already posted to db
	txn := db.NewTransaction(true)
	defer txn.Discard()
	for _, p := range allCurrentPosts {
		key := "post+" + p.URL
		err = txn.Set([]byte(key), []byte(p.URL))
		if err != nil {
			log.Fatal(fmt.Errorf("could not set current posto db: %w", err))
		}
	}
	// Commit the transaction and check for error.
	if err := txn.Commit(); err != nil {
		log.Fatal(fmt.Errorf("could not commit current posts to db: %w", err))
	}

	// Exec argument
	switch os.Args[1] {
	case "rss":
		feedConfigsJSON, err := os.ReadFile(path.Join(binaryPath, "rss_feeds.json"))
		if err != nil {
			log.Fatal("could not open 'rss_feeds.json", err)
		}
		feedConfigs := make([]RSSFeedConfig, 0)
		err = json.Unmarshal(feedConfigsJSON, &feedConfigs)
		if err != nil {
			log.Fatal("could not unmarshal feed configs:", err)
		}

		for _, feedConfig := range feedConfigs {
			if feedConfig.Username == "" {
				log.Print("not username given for %q", feedConfig.URL)
				continue
			}
			rssPosts, err := rss.GetPostsFromRSS(feedConfig.URL)
			if err != nil {
				log.Print("could not GetPostsFromRSS for url %q: %s", feedConfig.URL, err)
				continue
			}

			if feedConfig.MaxItems != nil {
				if *feedConfig.MaxItems < len(rssPosts) {
					rssPosts = rssPosts[:*feedConfig.MaxItems-1]
				}
			}

			// Filter out the urls which already where posted
			rssPosts, err = posts.FilterAlreadyPosted(db, rssPosts)
			if err != nil {
				log.Print("could not FilterAlreadyPosted:", err)
				continue
			}

			if feedConfig.CheckTitle {
				rssPosts = posts.FilterPostsByAIKeywordsInTitle(rssPosts)
			}

			if feedConfig.CheckLinkContent {
				rssPosts, err = posts.FilterPostsByAIContent(db, rssPosts)
				if err != nil {
					log.Print("could not FilterPostsByAIContent:", err)
					continue
				}
			}

			// Post articles
			jwt, err := aiapipro.LoginUser(feedConfig.Username)
			if err != nil {
				log.Print("could not GetAuthenticateUserToken:", err)
				continue
			}
			for _, p := range rssPosts {
				err = aiapipro.NewPost(db, p, jwt)
				if err != nil {
					fmt.Println("could not NewPost", err)
					continue
				}
			}
		}

	case "upvote":
		fmt.Println("UPVOTE BOTS")
		for i := 0; i < 5; i++ {
			jwt, err := aiapipro.GetRandomAuthenticateUserToken(db)
			if err != nil {
				fmt.Println("could not GetRandomAuthenticateUserToken", err)
				return
			}

			for k, post := range allCurrentPosts {
				if k > 30 {
					break
				}
				// Only upvote with a 5% chance
				if rand.Intn(100) > 5 {
					//NO lucky, no upvote
					continue
				}

				err := aiapipro.UpvotePost(post.ID, jwt)
				if err != nil {
					fmt.Println("could not UpvotePost ", post.ID, err)
					continue
				}
				fmt.Println("UPVOTED", post.ID)
			}
		}
	default:
		log.Fatal("No valid command. Expect 'rss' or 'upvote'", os.Args[1])
	}
}
