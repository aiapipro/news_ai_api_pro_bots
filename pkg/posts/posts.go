package posts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Posts []Post

type Post struct {
	Title       string `json:"title"`
	Url         string `json:"url"`
	Description string `json:"body"`
	Excerpt     string `json:"-"`
	JWT         string `json:"-"`
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

func PostJSON(url string, in, out interface{}) error {
	inJson, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("could not marshal input: %w", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(inJson))
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read body: %w", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("status not 200, but %d. Body: %s", resp.StatusCode, string(respBody))
	}
	if out != nil {

		err = json.Unmarshal(respBody, out)
		if err != nil {
			return fmt.Errorf("could not unmarshal body: %w", err)
		}
	}
	return nil
}
