package main

import (
	"encoding/json"
	"encoding/xml"
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
	"sort"
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

// Sitemap represents the structure of the sitemap
type NewsSitemap struct {
	XMLName xml.Name  `xml:"urlset"`
	XMLNS   string    `xml:"xmlns,attr"`
	NewsNS  string    `xml:"xmlns:news,attr"`
	URLs    []NewsURL `xml:"url"`
}

// NewsURL represents the structure of a news URL in the sitemap
type NewsURL struct {
	Loc     string   `xml:"loc"`
	LastMod string   `xml:"lastmod"`
	News    NewsInfo `xml:"news:news"`
}

// NewsInfo represents the news information in the sitemap
type NewsInfo struct {
	XMLName         xml.Name `xml:"news:news"`
	Publication     PublicationInfo
	PublicationDate string `xml:"news:publication_date"`
	Title           string `xml:"news:title"`
}

// PublicationInfo represents the publication information in the sitemap
type PublicationInfo struct {
	XMLName  xml.Name `xml:"news:publication"`
	Name     string   `xml:"news:name"`
	Language string   `xml:"news:language"`
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
	case "sitemap":
		sitemapPath := "sitemap.xml"
		if len(os.Args) > 2 {
			sitemapPath = os.Args[2]
		}
		newsSitemap, err := os.Create(sitemapPath)
		if err != nil {
			log.Fatal("Could not create sitemap.xml", err)
		}

		sort.Slice(allCurrentPosts, func(i, j int) bool {
			return allCurrentPosts[i].ID > allCurrentPosts[j].ID
		})
		newsUrls := make([]NewsURL, len(allCurrentPosts))
		for k, p := range allCurrentPosts {
			publishedDate, err := time.Parse("2006-01-02T15:04:05.999999", p.Published)
			if err != nil {
				log.Println("could not parse published date", p.Published, err)
				continue
			}
			if p.Counts.NewestCommentTime == "" {
				p.Counts.NewestCommentTime = p.Published
			}
			lastCommentDate, err := time.Parse("2006-01-02T15:04:05.999999", p.Counts.NewestCommentTime)
			if err != nil {
				log.Println("could not parse NewestCommentTime date", p.Published, err)
				continue
			}
			newsUrls[k] = NewsURL{
				Loc:     p.ApID,
				LastMod: lastCommentDate.Format("2006-01-02"),
				News: NewsInfo{
					Publication: PublicationInfo{
						Name:     "AI News (AI API Pro)",
						Language: "en",
					},
					PublicationDate: publishedDate.Format("2006-01-02"),
					Title:           p.Name,
				},
			}
		}

		// Create a sample Sitemap with multiple URLs
		sitemap := NewsSitemap{
			XMLNS:  "http://www.sitemaps.org/schemas/sitemap/0.9",
			NewsNS: "http://www.google.com/schemas/sitemap-news/0.9",
			URLs:   newsUrls,
		}

		// Marshal the struct into XML
		xmlData, err := xml.MarshalIndent(sitemap, "", "  ")
		if err != nil {
			log.Fatal("Could not marshal XML:", err)
		}

		// Print the XML
		_, err = newsSitemap.WriteString(xml.Header + string(xmlData))
		if err != nil {
			log.Fatal("Could not write to sitemap.xml", err)
		}

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
