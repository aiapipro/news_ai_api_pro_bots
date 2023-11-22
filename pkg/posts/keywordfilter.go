package posts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"jaytaylor.com/html2text"
)

var aiKeywords = []string{
	" ML ", " AI ", " AI-", " GAN ", " KNN ", " NLP ", " CNN ", "LLM",
	" Machine Learning ", " Artificial Intelligence ", " Neural Networks ", " Deep Learning ",
	" Data Science ", " Algorithm ", " Automation ", " Predictive Modeling ",
	" Natural Language Processing ", " Reinforcement Learning ",
	" Supervised Learning ", " Unsupervised Learning ", " Semi-Supervised Learning ",
	" Ensemble Learning ", " Transfer Learning ", " Convolutional Neural Networks ",
	" Recurrent Neural Networks ", " Generative Adversarial Networks ",
	" Feature Engineering ", " Gradient Descent ", " Overfitting ", " Bias-Variance Tradeoff ",
	" Hyperparameters ", " Backpropagation ",
	" ChatGPT ", "GPT", "llama2", "llama", "PaLM", " BART ",
}

func init() {
	for _, keyword := range aiKeywords {
		clean := strings.TrimSpace(keyword)
		aiKeywords = append(aiKeywords, " "+clean)
		aiKeywords = append(aiKeywords, clean+" ")
	}

}

func FilterPostsByAIKeywordsInTitle(rssPosts Posts) Posts {
	filteredPosts := make(Posts, 0, len(rssPosts))

	for _, p := range rssPosts {
		for _, keyword := range aiKeywords {
			if strings.Contains(strings.ToLower(p.Title), strings.ToLower(keyword)) {
				filteredPosts = append(filteredPosts, p)
				break
			}
		}
	}

	log.Printf("Filtered out %d in 'FilterPostsByAIKeywordsInTitle'", len(rssPosts)-len(filteredPosts))

	return filteredPosts
}

func FilterPostsByTitleRegex(rssPosts Posts, reg *regexp.Regexp, match bool) Posts {
	filteredPosts := make(Posts, 0, len(rssPosts))

	for _, p := range rssPosts {
		regMatched := reg.MatchString(p.Title)
		if (match && !regMatched) || (!match && regMatched) {
			// Regex not matched
			continue
		}
		filteredPosts = append(filteredPosts, p)
	}

	log.Printf("Filtered out %d in 'FilterPostsByTitleRegex'", len(rssPosts)-len(filteredPosts))

	return filteredPosts
}

var urlRegex = regexp.MustCompile(`https?:\/\/.*?\s`)

func EnrichPostsWithExcerpt(posts Posts) (Posts, error) {
	enrichedPosts := make(Posts, 0, len(posts))

	for _, p := range posts {
		resp, err := http.Get(p.Url)
		if err != nil {
			log.Println(fmt.Errorf("could not http get url %q: %w", p.Url, err))
			continue
		}
		plain, err := html2text.FromReader(resp.Body, html2text.Options{
			OmitLinks: true,
			TextOnly:  true,
		})
		if err != nil {
			log.Println(fmt.Errorf("could not read html from resp.Body %q: %w", p.Url, err))
			continue
		}
		//plain = urlRegex.ReplaceAllString(plain, "")
		if plain == "" {
			continue
		}

		excerpt := plain
		if words := strings.Split(excerpt, " "); len(words) > 350 {
			excerpt = strings.Join(words[:350], " ")
		}
		p.Excerpt = excerpt

		enrichedPosts = append(enrichedPosts, p)
	}

	return enrichedPosts, nil

}
func FilterPostsByAIContent(db *badger.DB, posts Posts) (Posts, error) {
	filteredPosts := make(Posts, 0, len(posts))

	txn := db.NewTransaction(true)
	defer txn.Discard()

	for _, p := range posts {
		key := "post+" + p.Url

		// Check again if we find keyword in body. Try to reduce GPT cost
		keywordFound := false
		for _, keyword := range aiKeywords {
			if strings.Contains(strings.ToLower(p.Excerpt), strings.ToLower(keyword)) {
				keywordFound = true
				break
			}
		}
		if !keywordFound {
			continue
		}

		if p.Excerpt == "" {
			continue
		}

		articleIsAboutAI, err := promptBetterCheckArticle(p.Excerpt)
		if err != nil {
			return nil, fmt.Errorf("could not promptBetterCheckArticle: %w", err)
		}
		if !articleIsAboutAI {
			// Not about AI
			err = txn.Set([]byte(key), []byte(p.Url))
			if err != nil {
				log.Println(fmt.Errorf("could not set to db: %w", err))
			}
			continue
		}

		filteredPosts = append(filteredPosts, p)

	}

	// Commit the transaction and check for error.
	if err := txn.Commit(); err != nil {
		log.Println(fmt.Errorf("could not commit to db: %w", err))
	}
	log.Printf("Filtered out %d in 'FilterPostsByAIContent'", len(posts)-len(filteredPosts))

	return filteredPosts, nil
}

type pbCheckArticlePayload struct {
	ArticleText string `json:"article_text"`
}
type pbResponse struct {
	Data string `json:"data"`
}

var promptBetterToken = "6kS38IwhXp3LZPWiXkb42MW84eJlNnMg4OYXXcfb"

var nonNumericRegex = regexp.MustCompile(`[^0-9.]`)

func promptBetterCheckArticle(articleExcerpt string) (bool, error) {
	payloadBytes, err := json.Marshal(pbCheckArticlePayload{
		ArticleText: articleExcerpt,
	})
	if err != nil {
		return false, fmt.Errorf("could not marshal input body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.promptbetter.ai/v1/2qcutndk/run/check-if-post-is-about-ai", bytes.NewReader(payloadBytes))
	if err != nil {
		return false, fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+promptBetterToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("could not create request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("could not read resp body: %w", err)
	}

	rData := pbResponse{}
	err = json.Unmarshal(respBody, &rData)
	if err != nil {
		return false, fmt.Errorf("could not unmarshal resp body: %w", err)
	}

	intResp := nonNumericRegex.ReplaceAllString(rData.Data, "")
	if intResp == "" {
		return false, nil
	}
	if strings.Contains(intResp, ".") {
		return false, nil
	}

	intRating, err := strconv.Atoi(intResp)
	if err != nil {
		return false, fmt.Errorf("could not convert response to int: %w", err)
	}

	return intRating > 5 && intRating <= 10, nil

}

type pbSummarizeArticlePayload struct {
	Title string `json:"title"`
	Post  string `json:"post"`
}

func PromptBetterSummarizeArticle(title, excerpt string) (string, error) {
	payloadBytes, err := json.Marshal(pbSummarizeArticlePayload{
		Title: title,
		Post:  excerpt,
	})
	if err != nil {
		return "", fmt.Errorf("could not marshal input body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.promptbetter.ai/v1/2qcutndk/run/write-summary-of-website", bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+promptBetterToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read resp body: %w", err)
	}

	rData := pbResponse{}
	err = json.Unmarshal(respBody, &rData)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal resp body: %w", err)
	}

	return rData.Data, nil

}

type pbRephraseTitlePayload struct {
	Title string `json:"title"`
	Excerpt string `json:"excerpt"`
}

func PromptBetterRephraseTitle(title, excerpt string) (string, error) {
	payloadBytes, err := json.Marshal(pbRephraseTitlePayload{
		Title: title,
		Excerpt: excerpt,
	})
	if err != nil {
		return "", fmt.Errorf("could not marshal input body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.promptbetter.ai/v1/2qcutndk/run/rephrase-title", bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+promptBetterToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read resp body: %w", err)
	}

	rData := pbResponse{}
	err = json.Unmarshal(respBody, &rData)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal resp body: %w", err)
	}

	return rData.Data, nil
}
