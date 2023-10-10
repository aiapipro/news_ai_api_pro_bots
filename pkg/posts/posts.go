package posts

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Posts []Post

type Post struct {
	Title string `json:"title"`
	Url   string `json:"url"`
}

func GetJSON(url string, out interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("could not http get: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read body: %w", err)
	}

	err = json.Unmarshal(respBody, out)
	if err != nil {
		return fmt.Errorf("could not unmarshal body: %w", err)
	}
	return nil
}
