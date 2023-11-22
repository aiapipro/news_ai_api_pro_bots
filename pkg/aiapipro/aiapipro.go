package aiapipro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"newsbots/pkg/posts"
	"os"
	"sort"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

var usernames = []string{"Vernon", "Bevan", "Jacinta", "Habib", "Michel", "Luther", "Josslyn", "Otho", "Safiya", "Roxie", "Sarra", "Jayse", "Tully", "Sephora", "Kenza", "Nosson", "Sadee", "Hagen", "Anitra", "Willma", "Blanchard", "Malia", "Baron", "Neo", "Viviann", "Haydon", "Catherine", "Thalia", "Titan", "Kenya", "Harlin", "Ayden", "Kasandra", "Saxon", "Ulisses", "Zach", "Aly", "Henna", "Romana", "Rowan", "Carmela", "Remi", "Peter", "Aman", "Jocelynne", "Flo", "Clifton", "Scot", "Gerry", "Keyton", "Hong", "Quint", "Cheron", "Katelynn", "Kaven", "Elsworth", "Jenelle", "Fernando", "Vilas", "Susette", "Meda", "Windsor", "Karine", "Kamela", "Kristeen", "Kairi", "Saloni", "Janice", "Abel", "Christin", "Stewart", "Guilherme", "Marylu", "Reymundo", "Anton", "Kaleena", "Florida", "Quinten", "Zoi", "Eleni", "Gia", "Selmer", "Reuben", "Zaynab", "Justen", "Emi", "Filip", "Sherry", "Wendie", "Vannie", "Deron", "Nicklaus", "Hamilton", "Rebekah", "Sabas", "Pixie", "Belinda", "Estel", "Glenda", "Darnell", "Mart", "Takumi", "Ezell", "Emanuel", "Nabor", "Abdulaziz", "Josh", "Owen", "Noor", "Andriana", "Sesar", "Celestia", "Giovana", "Kamila", "Vana", "Marja", "Nihal", "Aedan", "Gabrielle", "Berlin", "Jaxson", "Diangelo", "Zachari", "Wendi", "Ayelet", "Oren", "Clarisa", "Theola", "Heidy", "Abella", "Jude", "Zaden", "Salley", "Marcelino", "Cesario", "Marcia", "Phelan", "Sherrell", "Pascale", "Stephane", "Kelvin", "Marilu", "Edwina", "Florentino"}

var passwordSuffix = os.Getenv("PASSWORD_SUFFIX")

func init() {
	rand.Seed(time.Now().UnixNano())
}

func GetRandomAuthenticateUserToken(db *badger.DB) (token string, err error) {
	// Get a random username
	randomUsername := strings.ToLower(usernames[rand.Intn(len(usernames))])

	txn := db.NewTransaction(true)
	defer txn.Discard()

	_, err = txn.Get([]byte(randomUsername))
	if err == nil {
		// Key found, return it
		log.Print("Key found, user exists. Login")
		return LoginUser(randomUsername)
	}
	if !errors.Is(err, badger.ErrKeyNotFound) {
		return "", fmt.Errorf("could not get from db: %w", err)
	}
	err = txn.Set([]byte(randomUsername), []byte(token))
	if err != nil {
		return "", fmt.Errorf("could not set to db: %w", err)
	}

	// Commit the transaction and check for error.
	if err := txn.Commit(); err != nil {
		return "", fmt.Errorf("could not commit to db: %w", err)
	}

	// Key not found, create new user
	jwt, err := createNewUser(randomUsername)
	if err != nil {
		return LoginUser(randomUsername)
	}
	token = jwt
	return token, nil

}

func GetAuthenticateUserToken(db *badger.DB, username string) (token string, err error) {
	txn := db.NewTransaction(true)
	defer txn.Discard()

	_, err = txn.Get([]byte(username))
	if err == nil {
		// Key found, return it
		return LoginUser(username)
	}
	if !errors.Is(err, badger.ErrKeyNotFound) {
		return "", fmt.Errorf("could not get from db: %w", err)
	}

	err = txn.Set([]byte(username), []byte(token))
	if err != nil {
		return "", fmt.Errorf("could not set to db: %w", err)
	}
	// Commit the transaction and check for error.
	if err := txn.Commit(); err != nil {
		return "", fmt.Errorf("could not commit to db: %w", err)
	}

	// Key not found, create new user
	jwt, err := createNewUser(username)
	if err != nil {
		return "", fmt.Errorf("could not createNewUser: %w", err)
	}
	token = jwt

	return token, nil

}

type createNewUserResponse struct {
	JWT string `json:"jwt"`
}
type createNewUserRequest struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	PasswordVerify string `json:"password_verify"`
	ShowNSFW       bool   `json:"show_nsfw"`
}

type getPostsResponse struct {
	Posts []struct {
		Post Post `json:"post"`
	} `json:"posts"`
}

type Post struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	URL               string `json:"url"`
	CreatorID         int    `json:"creator_id"`
	CommunityID       int    `json:"community_id"`
	Removed           bool   `json:"removed"`
	Locked            bool   `json:"locked"`
	Published         string `json:"published"`
	Deleted           bool   `json:"deleted"`
	Nsfw              bool   `json:"nsfw"`
	EmbedTitle        string `json:"embed_title"`
	EmbedDescription  string `json:"embed_description"`
	ThumbnailURL      string `json:"thumbnail_url"`
	ApID              string `json:"ap_id"`
	Local             bool   `json:"local"`
	LanguageID        int    `json:"language_id"`
	FeaturedCommunity bool   `json:"featured_community"`
	FeaturedLocal     bool   `json:"featured_local"`
}

func GetPosts() ([]Post, error) {
	respPosts := make([]Post, 0)

	for page := 1; ; page++ {
		resp := getPostsResponse{}
		err := posts.GetJSON(fmt.Sprintf("https://news.aiapipro.com/api/v3/post/list?limit=50&page=%d", page), &resp)
		if err != nil {
			return nil, fmt.Errorf("could not GetJSON: %w", err)
		}
		if len(resp.Posts) == 0 {
			break
		}
		for _, p := range resp.Posts {
			newPost := p.Post
			newPost.URL = strings.TrimPrefix(newPost.URL, "https://reader.aiapipro.com/?url=")
			respPosts = append(respPosts, newPost)
		}
	}

	sort.Slice(respPosts, func(i, j int) bool {
		return respPosts[i].ID < respPosts[j].ID
	})

	return respPosts, nil
}

type deletePostsResponsRequest struct {
	PostID  int    `json:"post_id"`
	Removed bool   `json:"removed"`
	Auth    string `json:"auth"`
}

func DeletePost(jwt string, postID int) error {
	req := deletePostsResponsRequest{
		PostID:  postID,
		Removed: true,
		Auth:    jwt,
	}
	return posts.PostJSON("https://news.aiapipro.com/api/v3/post/remove", &req, nil)
}

type upvotePostRequest struct {
	PostID int    `json:"post_id"`
	Score  int    `json:"score"`
	Auth   string `json:"auth"`
}

func UpvotePost(postID int, jwt string) (err error) {
	upvoteReq := upvotePostRequest{
		PostID: postID,
		Score:  1,
		Auth:   jwt,
	}
	upvoteReqJSON, err := json.Marshal(upvoteReq)
	if err != nil {
		return fmt.Errorf("could not marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://news.aiapipro.com/api/v3/post/like", bytes.NewReader(upvoteReqJSON))
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not do request: %w", err)
	}
	return nil
}

func createNewUser(username string) (jwt string, err error) {
	newUser := createNewUserRequest{
		Username:       username,
		Password:       username + passwordSuffix,
		PasswordVerify: username + passwordSuffix,
		ShowNSFW:       false,
	}
	newUserJSON, err := json.Marshal(newUser)
	if err != nil {
		return "", fmt.Errorf("could not marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://news.aiapipro.com/api/v3/user/register", bytes.NewReader(newUserJSON))
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not do request: %w", err)
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read response body: %w", err)
	}

	newUserResp := createNewUserResponse{}
	err = json.Unmarshal(bodyText, &newUserResp)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal response body: %w", err)
	}
	return newUserResp.JWT, nil
}

type loginUserRequest struct {
	UsernameOrEmail string `json:"username_or_email"`
	Password        string `json:"password"`
}

func LoginUser(username string) (jwt string, err error) {
	loginUser := loginUserRequest{
		UsernameOrEmail: username,
		Password:        username + passwordSuffix,
	}
	loginUserJSON, err := json.Marshal(loginUser)
	if err != nil {
		return "", fmt.Errorf("could not marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://news.aiapipro.com/api/v3/user/login", bytes.NewReader(loginUserJSON))
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not do request: %w", err)
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Did not get status 200, but %d. Body: %s", resp.StatusCode, string(bodyText))
	}

	newUserResp := createNewUserResponse{}
	err = json.Unmarshal(bodyText, &newUserResp)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal response body: %w", err)
	}
	return newUserResp.JWT, nil
}

type newPostRequest struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Auth        string `json:"auth"`
	CommunityID int    `json:"community_id"`
	Body        string `json:"body,omitempty"`
}

func NewPost(db *badger.DB, post posts.Post, jwt string) (err error) {
	newPost := newPostRequest{
		Name:        post.Title,
		URL:         post.Url,
		Auth:        jwt,
		CommunityID: 4,
		Body:        post.Description,
	}
	if strings.Contains(post.Url, "paperswithcode.com") || strings.Contains(post.Url, "arxiv.org") {
		newPost.CommunityID = 7 // Set to papers community
	}

	newPostJSON, err := json.Marshal(newPost)
	if err != nil {
		return fmt.Errorf("could not marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://news.aiapipro.com/api/v3/post", bytes.NewReader(newPostJSON))
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not do request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status returned not 200 != %d. Body: %s", resp.StatusCode, string(body))
	}

	txn := db.NewTransaction(true)
	defer txn.Discard()

	key := "post+" + post.Url

	err = txn.Set([]byte(key), []byte(post.Url))
	if err != nil {
		return fmt.Errorf("could not set to db: %w", err)
	}

	// Commit the transaction and check for error.
	if err := txn.Commit(); err != nil {
		return fmt.Errorf("could not commit to db: %w", err)
	}

	fmt.Println("POSTED", post.Title)

	return nil
}

func FilterAlreadyPosted(db *badger.DB, rssPosts posts.Posts) (posts.Posts, error) {
	allCurrentUrls := make(map[string]bool, 0)
	notPosted := make(posts.Posts, 0, len(rssPosts))
	for _, p := range rssPosts {
		key := "post+" + p.Url

		if _, posted := allCurrentUrls[p.Url]; posted {
			// Found in current page. Filter out
			continue
		}

		txn := db.NewTransaction(true)
		defer txn.Discard()
		_, err := txn.Get([]byte(key))
		if err == nil {
			// Key found, filter out
			continue
		}

		notPosted = append(notPosted, p)
	}

	log.Printf("Filtered out %d in 'FilterAlreadyPosted'", len(rssPosts)-len(notPosted))

	return notPosted, nil

}

func FilterTooMuchPosted(db *badger.DB, max int, rssPosts posts.Posts, allCurrentPosts []Post) (posts.Posts, error) {
	todayDate := time.Now().Format("2006-01-02")

	notPosted := make(posts.Posts, 0, len(rssPosts))

	for _, p := range rssPosts {
		postUrl, err := url.Parse(p.Url)
		if err != nil {
			log.Print("could not parse url:", p.Url, err)
			continue
		}

		alreadyPosts := 0
		for _, cP := range allCurrentPosts {
			if !strings.Contains(cP.Published, todayDate) {
				// Not today
				continue
			}
			if strings.Contains(cP.URL, postUrl.Host) {
				alreadyPosts++
			}
		}
		if alreadyPosts >= max {
			continue
		}

		notPosted = append(notPosted, p)
	}

	log.Printf("Filtered out %d in 'FilterTooMuchPosted'", len(rssPosts)-len(notPosted))

	return notPosted, nil

}
