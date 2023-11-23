package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"newsbots/pkg/aiapipro"
	"newsbots/pkg/posts"
	"newsbots/pkg/posts/rss"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type RSSFeedConfig struct {
	URL              string  `json:"url"`
	TitleRegex       *string `json:"title_regex"`
	TitleNotRegex    *string `json:"title_not_regex"`
	TitleRegexRemove *string `json:"title_regex_remove"`
	CheckTitle       bool    `json:"check_title"`
	CheckLinkContent bool    `json:"check_link_content"`
	Username         string  `json:"username"`
	MaxItems         *int    `json:"max_items,omitempty"`
	Spread           *int    `json:"spread,omitempty"`
	UseReader        bool    `json:"use_reader"`
}

type ModerareRules struct {
	ForbiddenTitleRegex []string `json:"forbidden_title_regex"`
	ForbiddenUrlRegex   []string `json:"forbidden_url_regex"`
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

		allRssPosts := make(posts.Posts, 0)
		for _, feedConfig := range feedConfigs {
			if feedConfig.Spread != nil {
				// Random check if we skip
				if rand.Intn(100) > *feedConfig.Spread {
					log.Print("Skip as of spread", feedConfig.URL)
				}
			}
			log.Println(feedConfig.URL)
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
					log.Printf("Got too many rss items %d, cut down to %d", len(rssPosts), *feedConfig.MaxItems)

					rssPosts = rssPosts[:*feedConfig.MaxItems-1]
				}
			}

			// Filter out posts, where too many where already posted
			rssPosts, err = aiapipro.FilterTooMuchPosted(db, 2, rssPosts, allCurrentPosts)
			if err != nil {
				log.Print("could not FilterTooMuchPosted:", err)
				continue
			}

			// Filter out the urls which already where posted
			rssPosts, err = aiapipro.FilterAlreadyPosted(db, rssPosts)
			if err != nil {
				log.Print("could not FilterAlreadyPosted:", err)
				continue
			}

			if feedConfig.CheckTitle {
				rssPosts = posts.FilterPostsByAIKeywordsInTitle(rssPosts)
			}

			if feedConfig.TitleRegex != nil {
				titleRegexp := regexp.MustCompile(*feedConfig.TitleRegex)
				rssPosts = posts.FilterPostsByTitleRegex(rssPosts, titleRegexp, true)
			}

			rssPosts, err = posts.EnrichPostsWithExcerpt(rssPosts)
			if err != nil {
				log.Print("could not EnrichPostsWithExcerpt:", err)
				continue
			}

			if feedConfig.CheckLinkContent {
				rssPosts, err = posts.FilterPostsByAIContent(db, rssPosts)
				if err != nil {
					log.Print("could not FilterPostsByAIContent:", err)
					continue
				}
			}

			if feedConfig.TitleRegexRemove != nil {
				r := regexp.MustCompile(*feedConfig.TitleRegexRemove)
				for k, p := range rssPosts {
					rssPosts[k].Title = r.ReplaceAllString(p.Title, "")
				}
			}

			// Post articles
			var jwt string
			if feedConfig.Username == "random" {
				jwt, err = aiapipro.GetRandomAuthenticateUserToken(db)
				if err != nil {
					log.Print("could not GetAuthenticateUserToken:", err)
					continue
				}
				if jwt == "" {
					log.Print("did get empty jwt")
					continue
				}
			} else {
				jwt, err = aiapipro.LoginUser(feedConfig.Username)
				if err != nil {
					log.Print("could not GetAuthenticateUserToken:", err)
					continue
				}
			}
			for _, p := range rssPosts {
				if feedConfig.UseReader {
					p.Url = fmt.Sprintf("https://reader.aiapipro.com/?url=%s", p.Url)
				}
				p.JWT = jwt

				allRssPosts = append(allRssPosts, p)
			}
		}

		rand.Shuffle(len(allRssPosts), func(i, j int) {
			allRssPosts[i], allRssPosts[j] = allRssPosts[j], allRssPosts[i]
		})

		for _, p := range allRssPosts {

			resp, err := http.Get(p.Url)
			if err != nil {
				log.Printf("could not get url %q: %s", p.Url, err)
				continue
			} else if resp == nil || resp.StatusCode != http.StatusOK {
				continue
			}

			p.Description, err = posts.PromptBetterSummarizeArticle(p.Title, p.Excerpt)
			if err != nil {
				log.Println(fmt.Errorf("could not promptBetterSummarizeArticle: %w", err))
				continue
			}
			p.Title, err = posts.PromptBetterRephraseTitle(p.Title, "")
			if err != nil {
				log.Println(fmt.Errorf("could not promptBetterRephraseTitle: %w", err))
				continue
			}

			err = aiapipro.NewPost(db, p, p.JWT)
			if err != nil {
				fmt.Println("could not NewPost", err)
				continue
			}
		}
	case "moderate":
		feedConfigsJSON, err := os.ReadFile(path.Join(binaryPath, "moderate_rules.json"))
		if err != nil {
			log.Fatal("could not open 'moderate_rules.json", err)
		}
		moderateRules := ModerareRules{}
		err = json.Unmarshal(feedConfigsJSON, &moderateRules)
		if err != nil {
			log.Fatal("could not unmarshal moderateRules:", err)
		}

		alreadyFoundTitle := make(map[string]bool, 0)
		alreadyFoundUrl := make(map[string]bool, 0)

		for k := len(allCurrentPosts) - 1; k >= 0; k-- {
			p := allCurrentPosts[k]
			if strings.TrimSpace(p.URL) == "" {
				continue
			}

			// Also check urls
			delete := false
			for _, r := range moderateRules.ForbiddenUrlRegex {
				if len(strings.ReplaceAll(r, " ", "_")) < 4 {
					log.Println("Skip too short url regex", r)
					continue
				}

				if strings.Contains(strings.ToLower(p.URL), strings.ToLower(r)) {
					log.Println("Delete because of url regex", p.URL)

					delete = true
					break
				}
			}

			if !delete {
				// Check titles regex
				for _, r := range moderateRules.ForbiddenTitleRegex {
					if len(strings.ReplaceAll(r, " ", "_")) < 4 {
						log.Println("Skip too short title regex", r)
						continue
					}
					if strings.Contains(strings.ToLower(p.Name), strings.ToLower(r)) {
						delete = true
						log.Println("Delete because of title regex", p.Name)
						break
					}
				}
			}

			if !delete {
				// Check if title matches. IF yes, delete
				_, delete = alreadyFoundUrl[p.URL]
				if !delete {
					_, delete = alreadyFoundTitle[p.Name]
					if delete {
						log.Println("Delete because of already found title", p.Name)
					}
				} else {
					log.Println("Delete because of already found url", p.URL)
				}

			}

			if delete {
				// Delete the post
				jwt, err := aiapipro.LoginUser("moderator_bot")
				if err != nil {
					log.Println("could not login 'moderator_bot' post", err)
					continue
				}
				err = aiapipro.DeletePost(jwt, p.ID)
				if err != nil {
					log.Println("could not delete post", p.ID, p.Name, err)
					continue
				}

			}
			alreadyFoundTitle[p.Name] = true
			alreadyFoundUrl[p.URL] = true
		}

	case "upvote":
		fmt.Println("UPVOTE BOTS")
		for i := 0; i < 4; i++ {
			jwt, err := aiapipro.GetRandomAuthenticateUserToken(db)
			if err != nil {
				fmt.Println("could not GetRandomAuthenticateUserToken", err)
				continue
			}

			for k, _ := range allCurrentPosts {
				post := allCurrentPosts[len(allCurrentPosts)-1-k]
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
