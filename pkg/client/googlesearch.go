package client

import (
	"io/ioutil"
	"net/http"
	"strings"
)

type GoogleSearch struct {
	serperAPIKey string
	client       *http.Client
}

func NewGoogleSearch(serperAPIKey string) *GoogleSearch {
	return &GoogleSearch{
		serperAPIKey: serperAPIKey,
		client:       &http.Client{},
	}
}

func (c *GoogleSearch) search(query string) (string, error) {
	url := "https://google.serper.dev/search"
	method := "POST"

	payload := strings.NewReader(`{"q":"` + query + `"}`)

	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		return "", err
	}
	req.Header.Add("X-API-KEY", c.serperAPIKey)
	req.Header.Add("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
