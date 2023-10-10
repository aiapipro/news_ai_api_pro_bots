package aiapipro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"newsbots/pkg/posts"
	"os"
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
		return LoginUser(randomUsername)
	}
	if !errors.Is(err, badger.ErrKeyNotFound) {
		return "", fmt.Errorf("could not get from db: %w", err)
	}
	// Key not found, create new user
	jwt, err := createNewUser(randomUsername)
	if err != nil {
		return "", fmt.Errorf("could not createNewUser: %w", err)
	}
	token = jwt
	err = txn.Set([]byte(randomUsername), []byte(token))
	if err != nil {
		return "", fmt.Errorf("could not set to db: %w", err)
	}

	// Commit the transaction and check for error.
	if err := txn.Commit(); err != nil {
		return "", fmt.Errorf("could not commit to db: %w", err)
	}

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
	// Key not found, create new user
	jwt, err := createNewUser(username)
	if err != nil {
		return "", fmt.Errorf("could not createNewUser: %w", err)
	}
	token = jwt
	err = txn.Set([]byte(username), []byte(token))
	if err != nil {
		return "", fmt.Errorf("could not set to db: %w", err)
	}

	// Commit the transaction and check for error.
	if err := txn.Commit(); err != nil {
		return "", fmt.Errorf("could not commit to db: %w", err)
	}

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
	resp := getPostsResponse{}
	err := posts.GetJSON("https://news.aiapipro.com/api/v3/post/list", &resp)
	if err != nil {
		return nil, fmt.Errorf("could not GetJSON: %w", err)
	}
	respPosts := make([]Post, len(resp.Posts))
	for k, p := range resp.Posts {
		respPosts[k] = p.Post
	}
	return respPosts, nil
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
}

func NewPost(db *badger.DB, post posts.Post, jwt string) (err error) {
	newPost := newPostRequest{
		Name:        post.Title,
		URL:         post.Url,
		Auth:        jwt,
		CommunityID: 4,
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

	return nil
}
